// SPDX-License-Identifier: Apache-2.0
// XiaoNexus on-device DeepSeek + native TTS; no ASR keyword routing, BACK or master mute.
package main

import (
 "context"
 "crypto/rand"
 "crypto/subtle"
 "crypto/tls"
 "encoding/hex"
 "encoding/json"
 "errors"
 "flag"
 "fmt"
 "io"
 "log"
 "net"
 "net/http"
 "os"
 "os/exec"
 "os/signal"
 "path/filepath"
 "regexp"
 "strings"
 "sync"
 "sync/atomic"
 "syscall"
 "time"

 "xiaobot"
 "xiaobot/deepseekweb"
 "xiaobot/jarvis/openai"
 "xiaobot/runtimeenv"
 "xiaobot/voiceanswer"
)

const address = "127.0.0.1:18765"
const activity = "com.xiaomi.micolauncher/com.xiaomi.micolauncher.skills.baike.view.BaikeTextActivity"
var dialogPattern = regexp.MustCompile("^[a-fA-F0-9]{31,32}$")

type request struct {
 Kind string `json:"kind"`
 DialogID string `json:"dialog_id"`
 Query string `json:"query"`
 NativeAnswer string `json:"native_answer,omitempty"`
}
func (r request) validate() error {
 if r.Kind != "refusal" && r.Kind != "expand" && r.Kind != "direct" { return errors.New("invalid kind") }
 if !dialogPattern.MatchString(r.DialogID) { return errors.New("invalid dialog id") }
 n := len([]rune(strings.TrimSpace(r.Query)))
 if n == 0 || n > 600 { return errors.New("invalid query length") }
 if r.Kind == "expand" && strings.TrimSpace(r.NativeAnswer) == "" { return errors.New("missing previous answer") }
 if len([]rune(r.NativeAnswer)) > 1600 { return errors.New("previous answer too long") }
 if r.Kind != "expand" && r.NativeAnswer != "" { return errors.New("unexpected previous answer") }
 return nil
}
func answerQuery(r request) string {
 if r.Kind != "expand" { return r.Query }
 return "请重新、详细且准确地回答下面这个问题；先前小爱的回答仅供参考，可能过于简略或有误，请自行核实并纠正，不要只扩写旧答案。" +
     "\n原始问题：" + r.Query + "\n小爱之前的回答：" + r.NativeAnswer
}
func token(path string) (string, error) {
 if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil { return "", err }
 data, err := os.ReadFile(path)
 if errors.Is(err, os.ErrNotExist) {
  secret := make([]byte, 32)
  if _, err = rand.Read(secret); err != nil { return "", err }
  value := hex.EncodeToString(secret)
  f, ce := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
  if ce == nil {
   _, err = f.WriteString(value + "\n")
   if e := f.Close(); err == nil { err = e }
   if err != nil { return "", err }
   return value, nil
  }
  if !errors.Is(ce, os.ErrExist) { return "", ce }
  data, err = os.ReadFile(path)
 }
 if err != nil { return "", err }
 value := strings.TrimSpace(string(data))
 if len(value) != 64 { return "", errors.New("invalid token file") }
 _, err = hex.DecodeString(value)
 return value, err
}

type speechBox interface {
 MiTTSContext(context.Context,string) error
 RefreshMIoTCachedSID(context.Context) error
}
type server struct {
 admin *admin
 managementFault bool
 mu sync.Mutex
 ttsMu sync.Mutex
 authRefreshAt time.Time
 model atomic.Pointer[voiceanswer.Engine]
 boxMu sync.RWMutex
 boxNeedsAuth bool
 box speechBox
 doorbell *doorbellGuard
 generation uint64
 waitingDialog string
 cancel context.CancelFunc
 seen map[string]time.Time
 key string
}
func (s *server) speaker() speechBox {
 s.boxMu.RLock()
 defer s.boxMu.RUnlock()
 return s.box
}
func (s *server) setSpeaker(box speechBox) {
 s.preemptForDoorbell()
 s.boxMu.Lock()
 s.box = box
 s.boxNeedsAuth = false
 s.boxMu.Unlock()
}
func (s *server) speakerNeedsAuth() bool {
 s.boxMu.RLock()
 defer s.boxMu.RUnlock()
 return s.boxNeedsAuth
}
func (s *server) updateSpeakerAuth(box speechBox, needsAuth bool) {
 s.boxMu.Lock()
 defer s.boxMu.Unlock()
 if s.box == box { s.boxNeedsAuth = needsAuth }
}
func (s *server) routingPaused() bool {
 return s.managementFault || (s.admin != nil && s.admin.disabled.Load())
}
func (s *server) authorized(r *http.Request) bool {
 got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
 return len(got) == len(s.key) && subtle.ConstantTimeCompare([]byte(got), []byte(s.key)) == 1
}
func (s *server) handler(w http.ResponseWriter, r *http.Request) {
 if s.admin != nil && (r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/admin/")) { s.admin.serve(w,r); return }
 if !s.authorized(r) { http.Error(w, "unauthorized", http.StatusUnauthorized); return }
 if r.URL.Path == "/v1/update/voice" && r.Method == http.MethodPost && s.admin != nil {
  var mode string
  if !readAdminJSON(w,r,&mode) || (mode!="check" && mode!="apply" && mode!="version" && mode!="confirm"){http.Error(w,"invalid update request",400);return}
  dialog:=strings.ToLower(r.Header.Get("X-XiaoNexus-Dialog"))
  if dialog!="" && !dialogPattern.MatchString(dialog){http.Error(w,"invalid dialog",400);return}
  if s.doorbell!=nil && s.doorbell.check(r.Context()){http.Error(w,"doorbell prioritized",503);return}
  if mode=="version" {
   message := "无法读取当前版本。"
   if releaseVersion.MatchString(installedVersion) {
    spokenVersion := strings.ReplaceAll(strings.TrimPrefix(installedVersion, "V"), ".", "点")
    message = "XiaoNexus 当前版本是 " + spokenVersion + "。"
   }
   go s.announceUpdate(message)
   w.WriteHeader(http.StatusNoContent)
   return
  }
  s.admin.mu.Lock()
  expectedVersion:=""
  if mode=="confirm" {
   offer:=s.admin.voiceOffer
   if dialog=="" || dialog!=offer.dialog || !time.Now().Before(offer.expires) {
    s.admin.mu.Unlock()
    http.Error(w,"update confirmation expired",409)
    return
   }
   expectedVersion=offer.version
   mode="apply"
  }
  accepted:=s.admin.startUpdate(mode, s.announceUpdate, dialog, expectedVersion)
  busy:=s.admin.update.Busy
  pending:=s.admin.updatePending()
  s.admin.mu.Unlock()
  if !accepted{
   message:="暂时无法更新，请稍后再试。"
   if pending {
    message="更新已准备好，正在等待音箱空闲。请勿断电。"
   } else if busy {
    message="正在处理更新，请稍候。"
   }
   go s.announceUpdate(message)
   http.Error(w,"update unavailable",409)
   return
  }
  w.WriteHeader(http.StatusNoContent)
  return
 }
 if r.URL.Path == "/v1/health" && r.Method == http.MethodGet {
  w.Header().Set("Content-Type", "application/json")
  // The host must not arm refusal PCM if the personal doorbell receiver
  // would reject the follow-up DeepSeek request.
  healthy := !s.managementFault
  speakerReady := s.speaker() != nil && !s.speakerNeedsAuth()
  modelReady := s.model.Load() != nil
  blocked := healthy && s.doorbell != nil && s.doorbell.check(r.Context())
  paused := s.routingPaused()
  ready := healthy && speakerReady && modelReady && !blocked && !paused
  mode := "on-device-answer-only"
  if !speakerReady {
   mode = "receiver-unavailable"
  } else if !modelReady {
   mode = "model-not-configured"
  } else if paused {
   mode = "routing-paused"
  } else if blocked {
   mode = "doorbell-prioritized-or-state-unavailable"
  }
  s.mu.Lock()
  waiting := s.waitingDialog
  busy := s.cancel != nil
  s.mu.Unlock()
  var rules *adminRules
  offer:=""
  if s.admin != nil { rules = s.admin.rules.Load(); offer=s.admin.updateOfferDialog() }
  status := struct {
   Healthy bool `json:"healthy"`
   Ready bool `json:"ready"`
   RoutingBlocked bool `json:"routing_blocked"`
   Mode string `json:"mode"`
   NoGlobalMute bool `json:"no_global_mute"`
   AgentBusy bool `json:"agent_busy"`
   DoorbellBlocked bool `json:"doorbell_blocked"`
   UpdatePending bool `json:"update_pending"`
   UpdateOffer string `json:"update_offer,omitempty"`
   Rules *adminRules `json:"rules,omitempty"`
   WaitingDialog string `json:"waiting_dialog_id"`
  }{healthy, ready, blocked || paused || !modelReady || !speakerReady, mode, true, busy, blocked, s.admin != nil && s.admin.updateReadyToInstall(), offer, rules, waiting}
  _ = json.NewEncoder(w).Encode(status)
  return
 }
 if r.URL.Path != "/v1/query" || r.Method != http.MethodPost { http.NotFound(w, r); return }
 model := s.model.Load()
 if model == nil || s.speaker() == nil || s.speakerNeedsAuth() { http.Error(w, "not ready", http.StatusServiceUnavailable); return }
 var req request
 decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
 decoder.DisallowUnknownFields()
 if err := decoder.Decode(&req); err != nil { http.Error(w, "invalid json", http.StatusBadRequest); return }
 var extra interface{}
 if decoder.Decode(&extra) != io.EOF { http.Error(w, "extra json", http.StatusBadRequest); return }
 if req.validate() != nil { http.Error(w, "invalid request", http.StatusBadRequest); return }
 if s.doorbell != nil && s.doorbell.check(r.Context()) { http.Error(w, "doorbell prioritized", http.StatusServiceUnavailable); return }
 s.mu.Lock()
 if s.routingPaused() {
  s.mu.Unlock(); http.Error(w,"routing paused",http.StatusServiceUnavailable); return
 }
 if req.Kind=="direct" && s.cancel!=nil {
  s.mu.Unlock(); http.Error(w,"answer in progress",http.StatusConflict); return
 }
 now := time.Now()
 for id, at := range s.seen { if now.Sub(at) > 10*time.Minute { delete(s.seen,id) } }
 if _, present := s.seen[req.DialogID]; present {
  s.mu.Unlock(); http.Error(w, "duplicate dialog", http.StatusConflict); return
 }
 s.seen[req.DialogID] = now
 if s.cancel != nil { s.cancel() }
 ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
 s.cancel = cancel; s.generation++
 s.waitingDialog = strings.ToLower(req.DialogID)
 generation := s.generation
 s.mu.Unlock()
 s.admin.record("answer_accepted")
 w.Header().Set("Content-Type", "application/json")
 w.WriteHeader(http.StatusAccepted)
 io.WriteString(w, `{"accepted":true}`)
 go s.answer(ctx, generation, req, model)
}
func (s *server) preemptForDoorbell() {
 s.mu.Lock()
 s.generation++
 if s.cancel != nil { s.cancel() }
 s.cancel = nil
 s.waitingDialog = ""
 s.mu.Unlock()
 // No master mute state exists in this Agent.
}
func (s *server) finishWaiting(generation uint64) {
 s.mu.Lock()
 defer s.mu.Unlock()
 if s.generation == generation { s.waitingDialog = "" }
}
func (s *server) announceUpdate(message string) {
 ctx,cancel:=context.WithTimeout(context.Background(),8*time.Second)
 defer cancel()
 if s.doorbell!=nil && s.doorbell.check(ctx){return}
 if err:=s.nativeSpeech(ctx,message);err!=nil{log.Print("update voice feedback rejected")}
}
func (s *server) nativeSpeech(ctx context.Context,text string) error {
 s.ttsMu.Lock();defer s.ttsMu.Unlock()
 if err:=ctx.Err();err!=nil{return err}
 box:=s.speaker()
 if box==nil{return errors.New("speaker unavailable")}
 err:=box.MiTTSContext(ctx,text)
 if err==nil {
  s.updateSpeakerAuth(box,false)
  return nil
 }
 if ctx.Err()!=nil||!strings.Contains(err.Error(),"HTTP 401;"){return err}
 if !s.authRefreshAt.IsZero()&&time.Since(s.authRefreshAt)<5*time.Minute{return err}
 s.authRefreshAt=time.Now()
 log.Print("MIOT_TTS_AUTH401_CACHED_PASS_REFRESH_ONCE")
 if refreshErr:=box.RefreshMIoTCachedSID(ctx);refreshErr!=nil{
  if strings.Contains(refreshErr.Error(),"cached passToken rejected") ||
     strings.Contains(refreshErr.Error(),"verified cached passToken required") {
   s.updateSpeakerAuth(box,true)
  }
  return refreshErr
 }
 if err:=ctx.Err();err!=nil{return err}
 err=box.MiTTSContext(ctx,text)
 if ctx.Err()==nil && (err==nil || strings.Contains(err.Error(),"HTTP 401;")) {
  s.updateSpeakerAuth(box,err!=nil)
 }
 return err
}
func (s *server) answer(ctx context.Context, generation uint64, req request, model *voiceanswer.Engine) {
 defer func(){
  s.mu.Lock()
  if s.generation == generation { s.cancel = nil; s.waitingDialog = "" }
  s.mu.Unlock()
 }()
 start := time.Now()
 result, err := model.Answer(ctx, answerQuery(req))
 var full string
 if err != nil { s.admin.record("answer_failed"); full = "DeepSeek 请求没有成功，请稍后重试。" } else { full = strings.TrimSpace(result.Text) }
 if full == "" || ctx.Err() != nil { return }
 if s.doorbell != nil && s.doorbell.check(ctx) { s.preemptForDoorbell(); return }
 s.mu.Lock()
 valid := s.generation == generation && ctx.Err() == nil
 s.mu.Unlock()
 if !valid { return }
 s.finishWaiting(generation)
 // OEM BaikeTextActivity must start before TTS; never emit BACK.
 command := exec.CommandContext(ctx, "/system/bin/am", "start", "-n", activity,
  "--es", "KEY_TEXT_TITLE", "DeepSeek", "--es", "KEY_TEXT_CONTENT", full)
 if err := command.Run(); err != nil { log.Print("native full-screen activity unavailable; speech continues") }
 if ctx.Err() != nil { return }
 speech := strings.NewReplacer("**", "", "#", "", "`", "", "\n", " ").Replace(full)
 if err := s.nativeSpeech(ctx, speech); err != nil {
  s.admin.record("tts_rejected")
  log.Print("native TTS rejected; answer may be displayed")
 } else {
  s.admin.record("tts_accepted")
  log.Printf("DeepSeek TTS accepted mode=%s elapsed=%s", req.Kind, time.Since(start).Round(time.Millisecond))
 }
}
func main() {
 config := flag.String("config", "/data/adb/xiaonexus/config.json", "owner-only Xiaomi and DeepSeek configuration")
 secretPath := flag.String("token", "/data/adb/xiaonexus/control.token", "owner-only loopback token")
 flag.Parse()
 key, err := token(*secretPath)
 if err != nil { log.Fatal("control token failed: ", err) }
 xiaobot.FixedZone()
 if err := runtimeenv.ConfigureDNSFromEnv(); err != nil { log.Fatal("invalid private DNS configuration") }
 cfg, err := xiaobot.NewConfigFromFile(*config)
 if err != nil || cfg == nil {
  cfg = &xiaobot.Config{Hardware: "LX04", UseCommand: true}
  if err != nil && !errors.Is(err, os.ErrNotExist) { log.Print("Existing config could not be loaded") }
 }
 xiaobot.ParseHardwareCommandDict("xiaomi.ini")
 cfg.TokenPath = filepath.Join(filepath.Dir(*config), ".mi.token")
 s := &server{seen: make(map[string]time.Time), key: key}
 xiaomi := &xiaomiSetup{dir: filepath.Dir(*secretPath), base: *cfg, server: s}
 applyModel := func(settings adminModel, legacy bool) {
  if settings.Key == "" {
   s.model.Store(nil)
   return
  }
  client := openai.NewClient(settings.Key, settings.Endpoint, "")
  client.GPTOptions = cfg.GPTOptions
  searchKey := ""
  if legacy || settings.officialDeepSeek() { searchKey = settings.Key }
  s.model.Store(&voiceanswer.Engine{Chat: client, Search: deepseekweb.NewClient(searchKey), Model: settings.Name, Prompt: cfg.Prompt})
 }
 model := adminModel{Endpoint: cfg.OpenAIBackend, Name: cfg.Bot, Key: cfg.OpenAIKey}
 if model.Key == "" {
  if model.Endpoint == "" { model.Endpoint = "https://api.deepseek.com" }
  if model.Name == "" || model.Name == "gpt" { model.Name = "deepseek-flash" }
 }
 applyModel(model, true)
 s.admin, err = newAdmin(filepath.Dir(*secretPath), s.preemptForDoorbell)
 if err != nil {
  s.managementFault = true
  log.Print("management unavailable; routing paused")
 } else {
  s.admin.xiaomi = xiaomi
  s.admin.model = model
  s.admin.applyModel = applyModel
  if err := s.admin.loadModel(); err != nil { log.Print("management model settings invalid; using existing configuration") }
 }
 s.doorbell = newDoorbellGuard("http://127.0.0.1:7979/state")
 ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
 defer stop()
 if s.doorbell != nil { go s.doorbell.watch(ctx, func(){ s.preemptForDoorbell(); s.admin.record("doorbell_blocked") }) }
 listener, err := net.Listen("tcp", address)
 if err != nil { log.Fatal("loopback listen failed: ", err) }
 srv := &http.Server{Handler: http.HandlerFunc(s.handler), ReadHeaderTimeout: 3*time.Second, ReadTimeout: 8*time.Second, WriteTimeout: 10*time.Second}
 go xiaomi.restore(ctx)
 var lan *http.Server
 if s.admin != nil {
  lanListener, err := net.Listen("tcp4", lanAdminAddress)
  if err != nil {
   log.Print("LAN management listener unavailable: ", err)
  } else {
   lan = &http.Server{Handler: http.HandlerFunc(s.admin.serveLAN), ReadHeaderTimeout: 3*time.Second, ReadTimeout: 8*time.Second, WriteTimeout: 10*time.Second}
   go func(){
    cert := filepath.Join(filepath.Dir(*secretPath), "admin-tls.crt")
    privateKey := filepath.Join(filepath.Dir(*secretPath), "admin-tls.key")
    defer lanListener.Close()
    var err error
    _, certErr := os.Stat(cert)
    _, keyErr := os.Stat(privateKey)
    if errors.Is(certErr, os.ErrNotExist) && errors.Is(keyErr, os.ErrNotExist) {
     err = lan.Serve(lanListener)
    } else {
     pair, loadErr := tls.LoadX509KeyPair(cert, privateKey)
     if loadErr != nil {
      log.Print("LAN HTTPS certificate unavailable: ", loadErr)
      return
     }
     lan.TLSConfig = &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}
     redirect := &http.Server{Handler: http.HandlerFunc(redirectLANToHTTPS), ReadHeaderTimeout: 3*time.Second, WriteTimeout: 5*time.Second}
     redirectListener, redirectErr := net.Listen("tcp4", "0.0.0.0:80")
     if redirectErr != nil {
      log.Print("LAN HTTP redirect unavailable: ", redirectErr)
     } else {
      defer redirectListener.Close()
      defer redirect.Close()
      go func() {
       if err := redirect.Serve(redirectListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
        log.Print("LAN HTTP redirect stopped: ", err)
       }
      }()
     }
     err = lan.ServeTLS(lanListener, "", "")
    }
    if err != nil && !errors.Is(err,http.ErrServerClosed) { log.Print("LAN management server stopped: ", err) }
   }()
  }
 }
 go func(){
  <-ctx.Done()
  s.preemptForDoorbell()
  shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
  defer cancel()
  if lan != nil { _ = lan.Close() }
  _ = srv.Shutdown(shutdown)
 }()
 fmt.Println("XiaoNexus answer receiver ready on 127.0.0.1")
 if err := srv.Serve(listener); err != nil && !errors.Is(err,http.ErrServerClosed) { log.Fatal(err) }
}
