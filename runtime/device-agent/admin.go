// SPDX-License-Identifier: Apache-2.0
package main

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

const defaultAdminPassword = "XiaoNexus-LX04"
const adminCookie = "xiaonexus_admin"

//go:embed admin.html
var adminPage []byte

type adminCredential struct {
	Salt    []byte
	Hash    []byte
	Initial bool
}

type adminEvent struct {
	Time  time.Time `json:"time"`
	Event string    `json:"event"`
}

type admin struct {
	mu                         sync.Mutex
	dir                        string
	credential                 adminCredential
	session                    string
	expires                    time.Time
	nextLogin                  time.Time
	disabled                   atomic.Bool
	rules                      atomic.Pointer[adminRules]
	eventMu                    sync.Mutex
	events                     []adminEvent
	update                     adminUpdateStatus
	stop                       func()
	model                      adminModel
	applyModel                 func(adminModel, bool)
	xiaomi                     *xiaomiSetup
	testedModel, testedSpeaker string
	testedSpeakerBox           speechBox
}

func newAdmin(dir string, stop func()) (*admin, error) {
	a := &admin{dir: dir, stop: stop}
	file := filepath.Join(dir, "admin-password.json")
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		password := defaultAdminPassword
		if initial, readErr := os.ReadFile(filepath.Join(dir, "initial-password")); readErr == nil {
			password = strings.TrimSpace(string(initial))
			if len(password) < 20 || len(password) > 128 {
				return nil, errors.New("invalid initial password file")
			}
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return nil, readErr
		}
		if err := a.setPassword(password, true); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else if json.Unmarshal(data, &a.credential) != nil || len(a.credential.Salt) != 16 || len(a.credential.Hash) != 32 {
		return nil, errors.New("invalid admin credential")
	}
	data, err = os.ReadFile(filepath.Join(dir, "admin-routing.json"))
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, err
	case string(data) == "false":
		a.disabled.Store(true)
	case string(data) == "true":
	default:
		return nil, errors.New("invalid admin routing state")
	}
	if a.setupPending() {
		a.disabled.Store(true)
	}
	if err := a.loadRules(); err != nil {
		log.Print("management rules invalid; using built-in rules")
	}
	return a, nil
}

func saveAdminFile(dir, name string, data []byte) error {
	f, err := os.CreateTemp(dir, ".admin-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, name))
}

func passwordHash(password string, salt []byte) []byte {
	hash, _ := pbkdf2.Key(sha256.New, password, salt, 600000, 32)
	return hash
}

func (a *admin) setPassword(password string, initial bool) error {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	credential := adminCredential{salt, passwordHash(password, salt), initial}
	data, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	if err := saveAdminFile(a.dir, "admin-password.json", data); err != nil {
		return err
	}
	a.credential = credential
	a.session = ""
	return nil
}

func (a *admin) record(name string) {
	if a == nil {
		return
	}
	a.event(name)
}

func (a *admin) event(name string) {
	a.eventMu.Lock()
	defer a.eventMu.Unlock()
	if len(a.events) == 64 {
		a.events = append(a.events[:0], a.events[1:]...)
	}
	a.events = append(a.events, adminEvent{time.Now(), name})
}

func (a *admin) recentEvents() []adminEvent {
	a.eventMu.Lock()
	defer a.eventMu.Unlock()
	return append([]adminEvent(nil), a.events...)
}

func readAdminJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 12288))
	decoder.DisallowUnknownFields()
	return decoder.Decode(v) == nil && decoder.Decode(new(any)) == io.EOF
}

func (a *admin) serve(w http.ResponseWriter, r *http.Request) {
	if r.Host != "127.0.0.1:18765" && r.Host != "localhost:18765" {
		http.Error(w, "invalid host", http.StatusForbidden)
		return
	}
	a.serveManagement(w, r)
}

const lanAdminAddress = "0.0.0.0:8765"

func (a *admin) serveLAN(w http.ResponseWriter, r *http.Request) {
	local, ok := r.Context().Value(http.LocalAddrContextKey).(*net.TCPAddr)
	peerHost, _, err := net.SplitHostPort(r.RemoteAddr)
	peer := net.ParseIP(peerHost)
	if !ok || err != nil || !local.IP.IsPrivate() || !peer.IsPrivate() ||
		r.Host != local.String() || !sameLAN(local.IP, peer) {
		http.Error(w, "local network access only", http.StatusForbidden)
		return
	}
	if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/admin/") {
		http.NotFound(w, r)
		return
	}
	a.serveManagement(w, r)
}

func sameLAN(local, peer net.IP) bool {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, address := range addresses {
		subnet, ok := address.(*net.IPNet)
		if ok && subnet.IP.Equal(local) && subnet.Contains(peer) {
			return true
		}
	}
	return false
}

func (a *admin) serveManagement(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	if r.Method == http.MethodGet && r.URL.Path == "/" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(adminPage)
		return
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if r.Method == http.MethodPost && (r.Header.Get("Origin") != scheme+"://"+r.Host || r.Header.Get("Content-Type") != "application/json") {
		http.Error(w, "invalid origin or content type", http.StatusForbidden)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if r.Method == http.MethodPost && r.URL.Path == "/admin/login" {
		if a.credential.Initial && !protectedAdmin(r) {
			if _, err := os.Stat(filepath.Join(a.dir, "initial-password")); err == nil {
				http.Error(w, "请使用安装器提供的安全配置入口。", http.StatusForbidden)
				return
			}
		}
		if time.Now().Before(a.nextLogin) {
			http.Error(w, "try again shortly", http.StatusTooManyRequests)
			return
		}
		var body struct{ Password string }
		if !readAdminJSON(w, r, &body) || len(body.Password) > 256 {
			http.Error(w, "invalid password", http.StatusBadRequest)
			return
		}
		a.nextLogin = time.Now().Add(2 * time.Second)
		if subtle.ConstantTimeCompare(passwordHash(body.Password, a.credential.Salt), a.credential.Hash) != 1 {
			http.Error(w, "invalid password", http.StatusUnauthorized)
			return
		}
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			http.Error(w, "login failed", 500)
			return
		}
		a.xiaomi.cancel(a.session)
		a.testedModel, a.testedSpeaker = "", ""
		a.session = hex.EncodeToString(secret)
		a.expires = time.Now().Add(2 * time.Hour)
		http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: a.session, Path: "/admin/", Expires: a.expires, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode})
		a.event("login")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	cookie, err := r.Cookie(adminCookie)
	if err != nil || a.session == "" || time.Now().After(a.expires) || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(a.session)) != 1 {
		http.Error(w, "login required", http.StatusUnauthorized)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/admin/status":
		w.Header().Set("Content-Type", "application/json")
		update := a.update
		if a.updatePending() && !update.Busy {
			update.State = "pending"
		}
		_ = json.NewEncoder(w).Encode(struct {
			Version       string            `json:"version"`
			Initial       bool              `json:"initial"`
			Enabled       bool              `json:"enabled"`
			Events        []adminEvent      `json:"events"`
			Rules         *adminRules       `json:"rules"`
			Update        adminUpdateStatus `json:"update"`
			Model         adminModelStatus  `json:"model"`
			Xiaomi        xiaomiStatus      `json:"xiaomi"`
			Protected     bool              `json:"protected"`
			Setup         bool              `json:"setup"`
			ModelTested   bool              `json:"modelTested"`
			SpeakerTested bool              `json:"speakerTested"`
		}{installedVersion, a.credential.Initial, !a.disabled.Load(), a.recentEvents(), a.rules.Load(), update, a.model.status(), a.xiaomi.status(a.session), protectedAdmin(r), a.setupPending(), a.testedModel == a.session, a.speakerTested()})
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/admin/xiaomi/"):
		a.serveXiaomi(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/admin/speaker/test":
		a.testSpeaker(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/admin/model/test":
		a.testModel(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/admin/model":
		a.saveModel(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/admin/password":
		var body struct {
			Current  string `json:"current"`
			Password string `json:"password"`
			Confirm  string `json:"confirm"`
		}
		if !readAdminJSON(w, r, &body) {
			http.Error(w, "改密请求格式错误，请刷新管理页后重试。", http.StatusBadRequest)
			return
		}
		switch {
		case !a.credential.Initial && (body.Current == "" || len(body.Current) > 256):
			http.Error(w, "请填写当前密码。", http.StatusBadRequest)
			return
		case utf8.RuneCountInString(body.Password) < 8:
			http.Error(w, "新密码至少需要 8 个字符。", http.StatusBadRequest)
			return
		case len(body.Password) > 256:
			http.Error(w, "新密码不能超过 256 字节。", http.StatusBadRequest)
			return
		case strings.TrimSpace(body.Password) != body.Password:
			http.Error(w, "新密码的开头和末尾不能有空格或其他空白。", http.StatusBadRequest)
			return
		case body.Password == defaultAdminPassword:
			http.Error(w, "新密码不能设为出厂初始密码。", http.StatusBadRequest)
			return
		case body.Password != body.Confirm:
			http.Error(w, "两次输入的新密码不一致。", http.StatusBadRequest)
			return
		case !a.credential.Initial && body.Password == body.Current:
			http.Error(w, "新密码需要与当前密码不同。", http.StatusBadRequest)
			return
		}
		// Changing a password performs two PBKDF2 operations. On LX04 they
		// can exceed the server's normal 10-second response deadline.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Second))
		if !a.credential.Initial && subtle.ConstantTimeCompare(passwordHash(body.Current, a.credential.Salt), a.credential.Hash) != 1 {
			http.Error(w, "当前密码不正确。", http.StatusForbidden)
			return
		}
		if err := a.setPassword(body.Password, false); err != nil {
			http.Error(w, "save failed", 500)
			return
		}
		_ = os.Remove(filepath.Join(a.dir, "initial-password"))
		a.event("password_changed")
		w.WriteHeader(http.StatusNoContent)
	case (r.Method == http.MethodGet || r.Method == http.MethodPost) && r.URL.Path == "/admin/update/old-slots":
		a.serveOldUpdateSlots(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/admin/update/"):
		mode := strings.TrimPrefix(r.URL.Path, "/admin/update/")
		if mode != "check" && mode != "stage" && mode != "apply" {
			http.NotFound(w, r)
			return
		}
		if a.credential.Initial {
			http.Error(w, "change initial password", 403)
			return
		}
		if !a.startUpdate(mode) {
			http.Error(w, "update unavailable or already requested", 409)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == "/admin/logout":
		a.xiaomi.cancel(a.session)
		a.session = ""
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == "/admin/rules/test":
		if a.credential.Initial {
			http.Error(w, "change initial password", 403)
			return
		}
		var body struct {
			Rules adminRules `json:"rules"`
			Reply string     `json:"reply"`
		}
		if !readAdminJSON(w, r, &body) || body.Rules.validate() != nil || len(body.Reply) > 1024 {
			http.Error(w, "invalid test input", 400)
			return
		}
		sample := normalizeAdminReply(body.Reply)
		matched := false
		for _, rule := range body.Rules.Refusals {
			if sample != "" && sample == normalizeAdminReply(rule) {
				matched = true
				break
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"matched": matched})
	case r.Method == http.MethodPost && r.URL.Path == "/admin/rules":
		if a.credential.Initial {
			http.Error(w, "change initial password", http.StatusForbidden)
			return
		}
		var rules *adminRules
		if !readAdminJSON(w, r, &rules) || rules == nil || rules.validate() != nil {
			http.Error(w, "invalid rules", 400)
			return
		}
		data, _ := json.Marshal(rules)
		if err := saveAdminFile(a.dir, "admin-rules.json", data); err != nil {
			http.Error(w, "save failed", 500)
			return
		}
		a.rules.Store(rules)
		a.event("rules_saved")
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && r.URL.Path == "/admin/routing":
		if a.credential.Initial {
			http.Error(w, "change initial password", http.StatusForbidden)
			return
		}
		var enabled *bool
		if !readAdminJSON(w, r, &enabled) || enabled == nil {
			http.Error(w, "invalid routing state", 400)
			return
		}
		if *enabled && a.setupPending() {
			if !protectedAdmin(r) || a.xiaomi == nil || a.xiaomi.server.speaker() == nil || a.model.Key == "" ||
				!a.speakerTested() || a.testedModel != a.session {
				http.Error(w, "请先测试模型连接和音箱播报。", http.StatusConflict)
				return
			}
		}
		value := "true"
		if !*enabled {
			value = "false"
		}
		if err := saveAdminFile(a.dir, "admin-routing.json", []byte(value)); err != nil {
			http.Error(w, "save failed", 500)
			return
		}
		if *enabled && a.setupPending() {
			if err := os.Remove(filepath.Join(a.dir, "setup-pending")); err != nil {
				http.Error(w, "首次配置状态保存失败。", http.StatusInternalServerError)
				return
			}
		}
		a.disabled.Store(!*enabled)
		if *enabled {
			a.event("routing_enabled")
		} else {
			a.stop()
			a.event("routing_disabled")
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

const adminModelFile = "admin-model.json"

type adminModel struct {
	Endpoint string `json:"endpoint"`
	Name     string `json:"name"`
	Key      string `json:"key"`
}

type adminModelStatus struct {
	Endpoint string `json:"endpoint"`
	Name     string `json:"name"`
	KeySet   bool   `json:"keySet"`
}

func validModelEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" &&
		u.User == nil && !u.ForceQuery && u.RawQuery == "" && !strings.Contains(endpoint, "#") &&
		len(endpoint) <= 512
}

func (m adminModel) officialDeepSeek() bool {
	u, err := url.Parse(m.Endpoint)
	return err == nil && u.Scheme == "https" && strings.EqualFold(u.Host, "api.deepseek.com")
}

func (m adminModel) status() adminModelStatus {
	status := adminModelStatus{Name: m.Name, KeySet: m.Key != ""}
	if validModelEndpoint(m.Endpoint) {
		status.Endpoint = m.Endpoint
	}
	return status
}

func (m adminModel) valid() bool {
	return validModelEndpoint(m.Endpoint) && m.Name != "" && len(m.Name) <= 128 &&
		m.Key != "" && len(m.Key) <= 512 && !strings.ContainsAny(m.Key, " \t\r\n")
}

func (a *admin) loadModel() error {
	data, err := os.ReadFile(filepath.Join(a.dir, adminModelFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var model adminModel
	if json.Unmarshal(data, &model) != nil || !model.valid() {
		return errors.New("invalid model configuration")
	}
	a.model = model
	a.applyModel(model, false)
	return nil
}

func (a *admin) saveModel(w http.ResponseWriter, r *http.Request) {
	if a.credential.Initial {
		http.Error(w, "请先修改初始管理密码。", http.StatusForbidden)
		return
	}
	if a.applyModel == nil {
		http.Error(w, "模型设置暂不可用。", http.StatusServiceUnavailable)
		return
	}
	var model adminModel
	if !readAdminJSON(w, r, &model) {
		http.Error(w, "模型设置格式错误。", http.StatusBadRequest)
		return
	}
	model.Endpoint = strings.TrimSpace(model.Endpoint)
	model.Name = strings.TrimSpace(model.Name)
	model.Key = strings.TrimSpace(model.Key)
	if model.Key != "" && !protectedAdmin(r) {
		http.Error(w, "请通过安全配置入口填写新密钥。", http.StatusForbidden)
		return
	}
	if model.Key == "" && model.Endpoint == a.model.Endpoint {
		model.Key = a.model.Key
	}
	if !model.valid() {
		http.Error(w, "请填写 HTTPS API 地址和模型名称。首次设置或更换地址时，请填写对应的 API 密钥。", http.StatusBadRequest)
		return
	}
	data, _ := json.Marshal(model)
	if err := saveAdminFile(a.dir, adminModelFile, data); err != nil {
		http.Error(w, "模型设置保存失败。", http.StatusInternalServerError)
		return
	}
	a.model = model
	a.testedModel = ""
	a.applyModel(model, false)
	a.event("model_saved")
	w.WriteHeader(http.StatusNoContent)
}
