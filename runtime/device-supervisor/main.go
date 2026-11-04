// SPDX-License-Identifier: Apache-2.0
// A standalone, root-only LX04 supervisor using upstream frida-inject 16.6.6.
// No OEM APK modifications, BACK, global mute, ADB or desktop dependency.
package main

import (
 "bufio"
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "flag"
 "fmt"
 "io"
 "log"
 "net/http"
 "os"
 "os/exec"
 "os/signal"
 "path/filepath"
 "regexp"
 "strconv"
 "strings"
 "syscall"
 "time"
 "unicode"
)

const receiver = "http://127.0.0.1:18765"
var validID = regexp.MustCompile("^[0-9a-fA-F]{32}$")
// Action + reference intent recognition, NOT a list of complete voice sentences.
// Never automatically judge whether an OEM answer is "too short".
var expandActions = []string{
 "补充","完善","补全","展开","详细","细讲","细说","讲清楚","说清楚",
 "讲透","重讲","重新回答","重新解释","再回答","多讲","接着讲","接着说",
 "扩展","拓展","纠正","更正",
}
var priorReferences = []string{
 "刚才","刚刚","上一轮","上轮","上一个问题","上个问题",
 "之前","前面","那个回答","那个问题","你刚才",
}
var answerObjects = []string{
 "回答","答案","问题","问答","解释","内容","说的","讲的","那段","知识",
}
// Never preempt a compound device/music instruction to answer a follow-up.
var nativeActions = []string{
 "开灯","关灯","打开","关闭","播放","暂停","停止播放","音量",
 "下一首","上一首","退出","闹钟","定时","设置","门铃","米家","空调",
}
var shortExpandPattern = regexp.MustCompile(
 "^(再)?(补充|完善|补全|展开|展开讲讲|详细|细讲|细说|讲详细|说详细|详细讲|详细说|讲清楚|说清楚|重新回答|重新解释|多讲|接着讲)(一下|下|一点|点|一些|些|一遍|吧)?$",
)
func canon(s string) string {
 return strings.Map(func(r rune) rune {
  if unicode.IsSpace(r)||unicode.IsPunct(r)||unicode.IsSymbol(r) {return -1}
  return unicode.ToLower(r)
 },s)
}
func hasAny(s string, options []string)bool {
 for _,word:=range options {if strings.Contains(s,word){return true}}
 return false
}
func isBriefExpand(s string)bool {
 n:=canon(s)
 for i:=0;i<4;i++ {
  old:=n
  for _,p:=range []string{"请你","请","帮我","给我","小爱同学","小爱","让deepseek","deepseek","你"} {
   if strings.HasPrefix(n,p){n=strings.TrimPrefix(n,p);break}
  }
  if old==n {break}
 }
 return shortExpandPattern.MatchString(n)
}
func isExpand(s string) bool {
 n:=canon(s)
 if n==""||len([]rune(n))>60||strings.HasSuffix(n,"怎么样")||hasAny(n,nativeActions) {return false}
 if isBriefExpand(s) {return true}
 return hasAny(n,expandActions)&&hasAny(n,priorReferences)&&hasAny(n,answerObjects)
}

type instruction struct {kind, id, text string}
func parseOEM(line string) (instruction, bool) {
 pos:=strings.Index(line,"instruction=")
 if pos<0 {return instruction{},false}
 var raw struct {
  Header struct{ Namespace string `json:"namespace"`; Name string `json:"name"`; DialogID string `json:"dialog_id"` } `json:"header"`
  Payload struct{ Text string `json:"text"`; Final bool `json:"is_final"`; SkillIcon struct{ Description string `json:"description"` } `json:"skill_icon"`; Results []struct{ Text string `json:"text"` } `json:"results"` } `json:"payload"`
 }
 if json.Unmarshal([]byte(strings.TrimSpace(line[pos+len("instruction="):])), &raw)!=nil {return instruction{},false}
 id:=strings.ToLower(raw.Header.DialogID)
 if !validID.MatchString(id) {return instruction{},false}
 typ:=raw.Header.Namespace+"."+raw.Header.Name
 var kind, text string
 switch typ {
 case "SpeechRecognizer.RecognizeResult":
  if !raw.Payload.Final || len(raw.Payload.Results)==0 {return instruction{},false}
  kind="asr";text=raw.Payload.Results[0].Text
 case "Template.General":
  // Real OEM logs show Calculator and other skills also use Template.General.
  // Only the stock encyclopedic 智能问答 card is eligible for last-answer expansion.
  if raw.Payload.SkillIcon.Description!="智能问答"{return instruction{},false}
  kind="knowledge";text=raw.Payload.Text
 default:return instruction{},false
 }
 text=strings.TrimSpace(text)
 return instruction{kind,id,text},text!=""
}
type dialog struct {query string; at time.Time; expand bool; routed bool; nativeQuery string; nativeAnswer string}
type knowledge struct {dialogID,query,answer string; at time.Time}
type action struct{ kind,id,query,answer string }
type router struct {
 clock func()time.Time
 last string
 previous *knowledge
 dialogs map[string]*dialog
}
func newRouter(clock func()time.Time)*router{return &router{clock:clock,dialogs:make(map[string]*dialog)}}
func (r *router) feed(i instruction) action {
 now:=r.clock()
 for id,d:=range r.dialogs {if now.Sub(d.at)>5*time.Minute {delete(r.dialogs,id)}}
 if i.kind=="asr" {
  if _,exists:=r.dialogs[i.id];exists{return action{}}
  last:=r.last
  r.last=i.id
  // Bare "补充下" is accepted only immediately after an OEM knowledge card.
  // Long-form backward references may be used within the five-minute context.
  expand:=isExpand(i.text)
  if expand && isBriefExpand(i.text) {
   p:=r.previous
   expand=p!=nil && p.dialogID==last && !now.Before(p.at) && now.Sub(p.at)<=60*time.Second
  }
  r.dialogs[i.id]=&dialog{query:i.text,at:now,expand:expand}
  if len(r.dialogs)>64 {
   oldest:="";var at time.Time
   for id,d:=range r.dialogs {if oldest=="" || d.at.Before(at){oldest=id;at=d.at}}
   if oldest!=""{delete(r.dialogs,oldest)}
  }
  if expand {
   p:=r.previous
   if p!=nil && p.dialogID==last && now.Sub(p.at)>=0 && now.Sub(p.at)<=5*time.Minute {
    d:=r.dialogs[i.id];d.nativeQuery=p.query;d.nativeAnswer=p.answer
    // Do not submit DeepSeek yet: the OEM reply must first be gated.
    return action{kind:"arm_expand",id:i.id}
   }
   return action{kind:"expand_unavailable",id:i.id}
  }
  return action{kind:"arm",id:i.id}
 }
 if i.kind=="knowledge" && i.id==r.last {
  d:=r.dialogs[i.id]
  if d!=nil&&!d.expand {r.previous=&knowledge{dialogID:i.id,query:d.query,answer:i.text,at:now}}
 }
 return action{}
}
func (r *router) gate(id string)action {
 d:=r.dialogs[id]
 if d==nil||d.routed||r.clock().Sub(d.at)>12*time.Second{return action{}}
 if d.expand && (d.nativeQuery=="" || d.nativeAnswer==""){return action{}}
 d.routed=true
 if d.expand{return action{kind:"expand",id:id,query:d.nativeQuery,answer:d.nativeAnswer}}
 return action{kind:"refusal",id:id,query:d.query}
}

type client struct{ token string; http *http.Client}
func(c *client) request(ctx context.Context, path string, body interface{})bool {
 var reader io.Reader
 method:=http.MethodGet
 if body!=nil {
  buf,err:=json.Marshal(body);if err!=nil{return false}
  reader=bytes.NewReader(buf);method=http.MethodPost
 }
 req,err:=http.NewRequestWithContext(ctx,method,receiver+path,reader)
 if err!=nil{return false}
 req.Header.Set("Authorization","Bearer "+c.token)
 if body!=nil {req.Header.Set("Content-Type","application/json")}
 res,err:=c.http.Do(req);if err!=nil{return false}
 defer res.Body.Close()
 if body==nil {
  if res.StatusCode!=http.StatusOK{return false}
  var status struct{Ready bool `json:"ready"`;NoGlobalMute bool `json:"no_global_mute"`}
  if json.NewDecoder(io.LimitReader(res.Body,4096)).Decode(&status)!=nil{return false}
  return status.Ready&&status.NoGlobalMute
 }
 return res.StatusCode==http.StatusAccepted
}
func(c *client) healthy()bool {
 ctx,cancel:=context.WithTimeout(context.Background(),900*time.Millisecond);defer cancel()
 return c.request(ctx,"/v1/health",nil)
}
func(c *client) submit(a action)bool {
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second);defer cancel()
 body:=map[string]string{"kind":a.kind,"dialog_id":a.id,"query":a.query}
 if a.kind=="expand" {body["native_answer"]=a.answer}
 return c.request(ctx,"/v1/query",body)
}
func oemPID() (int,error) {
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second);defer cancel()
 out,err:=exec.CommandContext(ctx,"/system/bin/ps","-A").Output()
 if err!=nil{return 0,err}
 found:=0;pid:=0
 for _,line:=range strings.Split(string(out),"\n"){
  fields:=strings.Fields(line)
  if len(fields)<3 || fields[len(fields)-1]!="com.xiaomi.micolauncher"{continue}
  n,e:=strconv.Atoi(fields[1]);if e!=nil||n<=0{continue}
  found++;pid=n
 }
 if found!=1{return 0,fmt.Errorf("expected one OEM process, found %d",found)}
 return pid,nil
}
type event struct{kind,value string;inst instruction}
func scanLines(ctx context.Context,reader io.Reader,events chan<- event,kind string){
 scanner:=bufio.NewScanner(reader)
 scanner.Buffer(make([]byte,16*1024),1024*1024)
 for scanner.Scan(){
  line:=scanner.Text()
  if kind=="log"{
   if inst,ok:=parseOEM(line);ok{
    select{case events<-event{kind:"oem",inst:inst}:case <-ctx.Done():return}
   }
  } else {
   select{case events<-event{kind:"inject",value:line}:case <-ctx.Done():return}
  }
 }
 if ctx.Err()==nil{
  select{case events<-event{kind:"fatal",value:kind+" stream ended"}:case <-ctx.Done():}
 }
}
func parseGate(line string)(string,bool){
 // Normal Frida messages are JSON lines. Diagnostic console logs and
 // stdout handshake lines are intentionally separate and never treated
 // as routing decisions.
 var msg struct{Type string `json:"type"`;Payload struct{Kind string `json:"kind"`;DialogID string `json:"dialog_id"`} `json:"payload"`}
 if json.Unmarshal([]byte(line),&msg)!=nil || msg.Type!="send" ||
  msg.Payload.Kind!="gate_accepted" || !validID.MatchString(msg.Payload.DialogID){return "",false}
 return strings.ToLower(msg.Payload.DialogID),true
}
func run(ctx context.Context,injector,script string,enable bool,c *client)error{
 if !c.healthy(){return errors.New("answer receiver unhealthy; no injection")}
 pid,err:=oemPID();if err!=nil{return err}
 ctx,cancel:=context.WithCancel(ctx);defer cancel()
 inject:=exec.CommandContext(ctx,injector,"-p",strconv.Itoa(pid),"-s",script,"-i")
 stdout,err:=inject.StdoutPipe();if err!=nil{return err}
 stdin,err:=inject.StdinPipe();if err!=nil{return err}
 inject.Stderr=io.Discard // never store potentially sensitive OEM output
 if err:=inject.Start();err!=nil{return err}
 // Any later logcat/pipe failure must also detach the injector.
 defer func(){cancel();_ = stdin.Close();_ = inject.Process.Kill();_ = inject.Wait()}()
 logcat:=exec.CommandContext(ctx,"/system/bin/logcat","-v","raw","-T","1","MICO.cap:I","*:S")
 logOut,err:=logcat.StdoutPipe();if err!=nil{return err}
 logcat.Stderr=io.Discard
 if err:=logcat.Start();err!=nil{return err}
 defer func(){_ = logcat.Process.Kill();_ = logcat.Wait()}()
 events:=make(chan event,256)
 go scanLines(ctx,stdout,events,"inject")
 go scanLines(ctx,logOut,events,"log")
 r:=newRouter(time.Now)
 ready:=false
 tick:=time.NewTicker(5*time.Second);defer tick.Stop()
 for {
  select {
  case <-ctx.Done():return nil
  case <-tick.C:
   current,e:=oemPID()
   if e!=nil || current!=pid{return errors.New("OEM PID changed; detach and fail-open")}
   if !c.healthy() && enable{
    // The target script itself has strict 45s one-shot expiry.
    // On receiver failure stop this entire attach session immediately.
    return errors.New("answer receiver unavailable; detach and fail-open")
   }
  case ev:=<-events:
   if ev.kind=="fatal"{return errors.New(ev.value)}
   if ev.kind=="inject"{
    if ev.value=="XIAONEXUS_READY"{ready=true;log.Print("XiaoNexus injector READY_PASS_ALL");continue}
    if strings.Contains(ev.value,`"type":"error"`){return errors.New("Frida JavaScript exception")}
    if id,ok:=parseGate(ev.value);ok{
     if !enable || !c.healthy(){return errors.New("gate accepted but receiver not ready")}
     if a:=r.gate(id);(a.kind=="refusal"||a.kind=="expand") && !c.submit(a){
      return errors.New("accepted PCM gate but DeepSeek request failed")
     }
    }
   } else if ev.kind=="oem" {
    a:=r.feed(ev.inst)
    if a.kind==""||!ready||!enable{continue}
    if a.kind=="arm" || a.kind=="arm_expand"{
     if !c.healthy(){continue}
     command:="ARM "
     if a.kind=="arm_expand"{command="ARMEXP "}
     _,_ = io.WriteString(stdin,command+a.id+"\n")
    }
    if a.kind=="expand_unavailable"{log.Print("no immediately preceding OEM knowledge answer")}
   }
  }
 }
}
func singleton(path string)(*os.File,error){
 lock,err:=os.OpenFile(path,os.O_CREATE|os.O_RDWR,0600)
 if err!=nil{return nil,err}
 if err=syscall.Flock(int(lock.Fd()),syscall.LOCK_EX|syscall.LOCK_NB);err!=nil{
  lock.Close()
  return nil,fmt.Errorf("another XiaoNexus supervisor already owns the lock: %w",err)
 }
 return lock,nil
}
func main(){
 var injector=flag.String("injector","/data/adb/xiaonexus/frida/frida-inject","official Frida 16.6.6 Android ARM binary")
 var script=flag.String("script","/data/adb/xiaonexus/frida/lx04_nexus_gate.js","pinned XiaoNexus JavaScript")
 var tokenPath=flag.String("token","/data/adb/xiaonexus/control.token","owner-only receiver token")
 var enable=flag.Bool("enable",false,"enable real refusal PCM gating; default OBSERVE ONLY")
 flag.Parse()
 data,err:=os.ReadFile(*tokenPath)
 if err!=nil{log.Fatal("receiver token absent; fail-open")}
 secret:=strings.TrimSpace(string(data))
 // Kernel flock is released automatically even after a crash, avoiding stale PID locks.
 guard,lockErr:=singleton(filepath.Join(filepath.Dir(*tokenPath),"supervisor.lock"))
 if lockErr!=nil{log.Fatal("singleton supervisor: ",lockErr)}
 defer guard.Close()
 if len(secret)!=64{log.Fatal("invalid receiver token")}
 for _,ch:=range secret{if !strings.ContainsRune("0123456789abcdef",ch){log.Fatal("invalid receiver token")}}
 if _,err=os.Stat(*script);err!=nil{log.Fatal("Frida script unavailable")}
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second)
 out,err:=exec.CommandContext(ctx,*injector,"--version").Output();cancel()
 if err!=nil||strings.TrimSpace(string(out))!="16.6.6"{
  log.Fatal("requires official frida-inject 16.6.6 android-arm; no injection")
 }
 ctx,stop:=context.WithCancel(context.Background());defer stop()
 signals:=make(chan os.Signal,1)
 // No independent launcher restart or OEM kill: the caller owns the
 // process lifecycle and may restart with a bounded, external supervisor.
 signal.Notify(signals,syscall.SIGINT,syscall.SIGTERM)
 go func(){<-signals;stop()}()
 c:=&client{token:secret,http:&http.Client{Timeout:3*time.Second}}
 if !*enable {log.Print("XiaoNexus OBSERVE_ONLY: no PCM or Toast will be changed")}
 if err=run(ctx,*injector,*script,*enable,c);err!=nil{log.Fatal("XiaoNexus stopped (fail-open): ",err)}
}
