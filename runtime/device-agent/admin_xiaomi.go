// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"xiaobot"
	"xiaobot/jarvis/openai"
	"xiaobot/miservice"
)

const xiaomiConfigFile = "admin-xiaomi.json"

type xiaomiBinding struct {
	Account, Password, DID, TokenDirectory string
}

type xiaomiDevice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type xiaomiStatus struct {
	State   string         `json:"state"`
	Bound   bool           `json:"bound"`
	Method  string         `json:"method,omitempty"`
	Devices []xiaomiDevice `json:"devices,omitempty"`
}

type xiaomiAttempt struct {
	session string
	context.Context
	stop         context.CancelFunc
	otp          chan string
	status       xiaomiStatus
	selectDevice chan string
}

type xiaomiSetup struct {
	mu      sync.Mutex
	dir     string
	base    xiaobot.Config
	server  *server
	attempt *xiaomiAttempt
	next    time.Time
}

func (a *admin) setupPending() bool {
	_, err := os.Stat(filepath.Join(a.dir, "setup-pending"))
	return err == nil
}

func protectedAdmin(r *http.Request) bool {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	return r.TLS != nil || (ip != nil && ip.IsLoopback())
}

func (x *xiaomiSetup) restore(ctx context.Context) {
	cfg := x.base
	data, err := os.ReadFile(filepath.Join(x.dir, xiaomiConfigFile))
	if err == nil {
		var binding xiaomiBinding
		if json.Unmarshal(data, &binding) != nil ||
			!strings.HasPrefix(binding.TokenDirectory, "xiaomi-") ||
			filepath.Base(binding.TokenDirectory) != binding.TokenDirectory {
			return
		}
		cfg.Account, cfg.Password, cfg.MiDID = binding.Account, binding.Password, binding.DID
		cfg.Hardware = "LX04"
		cfg.TokenPath = filepath.Join(x.dir, binding.TokenDirectory, ".mi.token")
	} else if !errors.Is(err, os.ErrNotExist) {
		return
	}
	if cfg.Hardware != "LX04" || cfg.Account == "" || cfg.MiDID == "" {
		return
	}
	box := xiaobot.NewXiaoMi(&cfg)
	if err := box.InitMiBox(); err != nil || ctx.Err() != nil {
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	// A successful interactive binding always wins over startup restoration.
	if x.server.speaker() == nil {
		x.server.setSpeaker(box)
	}
}

func (x *xiaomiSetup) status(session string) xiaomiStatus {
	if x == nil {
		return xiaomiStatus{State: "unavailable"}
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	bound := x.server.speaker() != nil
	if x.attempt != nil && x.attempt.session == session {
		status := x.attempt.status
		status.Bound = bound
		return status
	}
	return xiaomiStatus{State: "idle", Bound: bound}
}

func (x *xiaomiSetup) cancel(session string) {
	if x == nil {
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.attempt != nil && x.attempt.session == session {
		x.attempt.stop()
		x.attempt = nil
	}
}

func (x *xiaomiSetup) start(session, account, password string) bool {
	x.mu.Lock()
	defer x.mu.Unlock()
	if time.Now().Before(x.next) || (x.attempt != nil && x.attempt.Err() == nil) {
		return false
	}
	x.next = time.Now().Add(15 * time.Second)
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Minute)
	attempt := &xiaomiAttempt{
		session: session, Context: ctx, stop: stop, otp: make(chan string, 1),
		selectDevice: make(chan string, 1), status: xiaomiStatus{State: "working"},
	}
	x.attempt = attempt
	go x.authorize(attempt, account, password)
	return true
}

func (x *xiaomiSetup) authorize(a *xiaomiAttempt, username, password string) {
	defer a.stop()
	state := func(value xiaomiStatus) {
		x.mu.Lock()
		defer x.mu.Unlock()
		a.status = value
	}
	dir, err := os.MkdirTemp(x.dir, "xiaomi-")
	if err != nil {
		state(xiaomiStatus{State: "save_failed"})
		return
	}
	saved := false
	defer func() {
		if !saved {
			_ = os.RemoveAll(dir)
		}
	}()
	tokenPath := filepath.Join(dir, ".mi.token")
	account := miservice.NewAccount(username, password, miservice.NewTokenStore(tokenPath))
	account.SetOTPProvider(func(method string) (string, error) {
		state(xiaomiStatus{State: "otp", Method: method})
		select {
		case code := <-a.otp:
			return code, nil
		case <-a.Done():
			return "", a.Err()
		}
	})
	devices, err := miservice.NewAIService(account).DeviceList(0)
	if err != nil {
		var login *miservice.LoginError
		switch {
		case a.Err() != nil:
			state(xiaomiStatus{State: "expired"})
		case errors.As(err, &login) && (login.CaptchaURL != "" || login.VerificationURL != ""):
			state(xiaomiStatus{State: "manual_verification"})
		case strings.Contains(err.Error(), "70022"):
			x.mu.Lock()
			x.next = time.Now().Add(5 * time.Minute)
			x.mu.Unlock()
			state(xiaomiStatus{State: "rate_limited"})
		default:
			state(xiaomiStatus{State: "login_failed"})
		}
		return
	}
	var choices []xiaomiDevice
	for _, device := range devices {
		if strings.EqualFold(device.Hardware, "LX04") && device.MiotDID != "" {
			name := device.Alias
			if name == "" {
				name = device.Name
			}
			if name == "" {
				name = "LX04"
			}
			choices = append(choices, xiaomiDevice{device.MiotDID, name})
		}
	}
	if len(choices) == 0 {
		state(xiaomiStatus{State: "no_devices"})
		return
	}
	state(xiaomiStatus{State: "select", Devices: choices})
	var did string
	select {
	case did = <-a.selectDevice:
	case <-a.Done():
		state(xiaomiStatus{State: "expired"})
		return
	}
	cfg := x.base
	cfg.Account, cfg.Password, cfg.MiDID = username, password, did
	cfg.Hardware, cfg.TokenPath = "LX04", tokenPath
	box := xiaobot.NewXiaoMi(&cfg)
	if box.InitMiBox() != nil || a.Err() != nil {
		state(xiaomiStatus{State: "device_failed"})
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.attempt != a || a.Err() != nil {
		return
	}
	binding := xiaomiBinding{username, password, did, filepath.Base(dir)}
	data, _ := json.Marshal(binding)
	if err := saveAdminFile(x.dir, xiaomiConfigFile, data); err != nil {
		a.status = xiaomiStatus{State: "save_failed"}
		return
	}
	saved = true
	x.server.setSpeaker(box)
	a.status = xiaomiStatus{State: "authorized"}
}

func (a *admin) serveXiaomi(w http.ResponseWriter, r *http.Request) {
	x := a.xiaomi
	if a.credential.Initial || x == nil || !protectedAdmin(r) {
		http.Error(w, "请先更改初始密码，并通过安全配置入口操作。", http.StatusForbidden)
		return
	}
	switch strings.TrimPrefix(r.URL.Path, "/admin/xiaomi/") {
	case "start":
		a.testedSpeaker = ""
		var input struct{ Account, Password string }
		if !readAdminJSON(w, r, &input) || strings.TrimSpace(input.Account) == "" ||
			len(input.Account) > 160 || input.Password == "" || len(input.Password) > 256 {
			http.Error(w, "请填写小米账号和密码。", http.StatusBadRequest)
			return
		}
		if !x.start(a.session, strings.TrimSpace(input.Account), input.Password) {
			http.Error(w, "授权正在进行，或重试过于频繁。", http.StatusConflict)
			return
		}
	case "otp", "select":
		var value string
		if !readAdminJSON(w, r, &value) {
			http.Error(w, "请求格式有误。", http.StatusBadRequest)
			return
		}
		if !x.submit(a.session, filepath.Base(r.URL.Path), strings.TrimSpace(value)) {
			http.Error(w, "请刷新授权状态并检查输入。", http.StatusConflict)
			return
		}
	case "cancel":
		x.cancel(a.session)
	default:
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (x *xiaomiSetup) submit(session, step, value string) bool {
	x.mu.Lock()
	defer x.mu.Unlock()
	a := x.attempt
	if a == nil || a.session != session || a.Err() != nil || a.status.State != step {
		return false
	}
	switch step {
	case "otp":
		if len(value) < 4 || len(value) > 8 || strings.Trim(value, "0123456789") != "" {
			return false
		}
		a.otp <- value
	case "select":
		valid := false
		for _, device := range a.status.Devices {
			valid = valid || device.ID == value
		}
		if !valid {
			return false
		}
		a.selectDevice <- value
	}
	a.status = xiaomiStatus{State: "working"}
	return true
}

func (a *admin) testModel(w http.ResponseWriter, r *http.Request) {
	if a.credential.Initial || !protectedAdmin(r) {
		http.Error(w, "请通过安全配置入口测试模型。", http.StatusForbidden)
		return
	}
	var model adminModel
	if !readAdminJSON(w, r, &model) {
		http.Error(w, "配置格式有误。", http.StatusBadRequest)
		return
	}
	model.Endpoint, model.Name, model.Key = strings.TrimSpace(model.Endpoint), strings.TrimSpace(model.Name), strings.TrimSpace(model.Key)
	if model.Key == "" && model.Endpoint == a.model.Endpoint {
		model.Key = a.model.Key
	}
	if !model.valid() {
		http.Error(w, "请填写 API 地址、模型和密钥。", http.StatusBadRequest)
		return
	}
	session := a.session
	// The caller holds the admin mutex. Release it for the outbound request.
	a.mu.Unlock()
	defer a.mu.Lock()
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(20 * time.Second))
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	client := openai.NewClient(model.Key, model.Endpoint, "")
	response, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: model.Name, Messages: []openai.ChatCompletionMessage{{Role: "user", Content: "请回复 OK"}}, MaxTokens: 64,
	})
	if err != nil || response == nil || len(response.Choices) == 0 {
		http.Error(w, "模型测试失败，请检查地址、模型名称、密钥及余额。", http.StatusBadGateway)
		return
	}
	a.mu.Lock()
	if a.session == session && a.model == model {
		a.testedModel = session
	}
	a.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (a *admin) testSpeaker(w http.ResponseWriter, r *http.Request) {
	if a.credential.Initial || a.xiaomi == nil || !protectedAdmin(r) {
		http.Error(w, "请通过安全配置入口测试播报。", http.StatusForbidden)
		return
	}
	s := a.xiaomi.server
	speaker := s.speaker()
	if speaker == nil {
		http.Error(w, "请先授权并选择音箱。", http.StatusConflict)
		return
	}
	session := a.session
	a.mu.Unlock()
	defer a.mu.Lock()
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(20 * time.Second))
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if s.doorbell != nil && s.doorbell.check(ctx) {
		http.Error(w, "门铃正在使用或状态不可用，请稍后试播。", http.StatusConflict)
		return
	}
	if s.nativeSpeech(ctx, "小爱音箱连接成功，这是一条播报测试。") != nil {
		http.Error(w, "试播失败，请检查音箱状态或重新授权。", http.StatusBadGateway)
		return
	}
	a.mu.Lock()
	if a.session == session && s.speaker() == speaker {
		a.testedSpeaker = session
	}
	a.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}
