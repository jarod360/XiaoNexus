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
 "regexp"
 "strconv"
 "strings"
 "syscall"
 "time"
)

const receiver = "http://127.0.0.1:18765"
var validID = regexp.MustCompile("^[0-9a-fA-F]{32}$")
var strictCommands = map[string]bool{
 "让deepseek完善一下小爱同学上一轮的回答":true,
 "让deepseek完善下小爱同学上轮的回答":true,
 "让deepseek补充一下小爱同学上一轮的回答":true,
 "让deepseek详细回答上一个问题":true,
 "让deepseek重新回答上一个问题":true,
 "deepseek完善一下刚才小爱的回答":true,
 "deepseek详细解释一下刚才那个问题":true,
}
func canon(s string) string {
 s = strings.Join(strings.Fields(s), "")
 return strings.ToLower(strings.TrimRight(strings.ReplaceAll(strings.ReplaceAll(s,",","，"),"!","！"),"。"))
}
func isExpand(s string) bool { return strictCommands[canon(s)] }

type instruction struct {kind, id, text string}
func parseOEM(line string) (instruction, bool) {
 pos:=strings.Index(line,"instruction=")
 if pos<0 {return instruction{},false}
 var raw struct {
  Header struct{ Namespace string `json:"namespace"`; Name string `json:"name"`; DialogID string `json:"dialog_id"` } `json:"header"`
  Payload struct{ Text string `json:"text"`; Final bool `json:"is_final"`; Results []struct{ Text string `json:"text"` } `json:"results"` } `json:"payload"`
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
 case "Template.General":kind="knowledge";text=raw.Payload.Text
 default:return instruction{},false
 }
 text=strings.TrimSpace(text)
 return instruction{kind,id,text},text!=""
}
type dialog struct {query string; at time.Time; expand bool; routed bool}
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
  expand:=isExpand(i.text)
  r.dialogs[i.id]=&dialog{query:i.text,at:now,expand:expand}
  if len(r.dialogs)>64 {
   oldest:="";var at time.Time
   for id,d:=range r.dialogs {if oldest=="" || d.at.Before(at){oldest=id;at=d.at}}
   if oldest!=""{delete(r.dialogs,oldest)}
  }
  if expand {
   p:=r.previous
   if p!=nil && p.dialogID==last && now.Sub(p.at)>=0 && now.Sub(p.at)<=5*time.Minute {
    return action{kind:"expand",id:i.id,query:p.query,answer:p.answer}
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
 if d==nil||d.routed||d.expand||r.clock().Sub(d.at)>12*time.Second{return action{}}
 d.routed=true
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
 logcat:=exec.CommandContext(ctx,"/system/bin/logcat","-v","raw","-T","1","MICO.cap:I","*:S")
 logOut,err:=logcat.StdoutPipe();if err!=nil{return err}
 logcat.Stderr=io.Discard
 if err:=logcat.Start();err!=nil{return err}
 defer func(){
  cancel()
  _=stdin.Close()
  _=inject.Process.Kill()
  _=logcat.Process.Kill()
  _=inject.Wait()
  _=logcat.Wait()
 }()
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
     if a:=r.gate(id);a.kind=="refusal" && !c.submit(a){
      return errors.New("accepted PCM gate but DeepSeek request failed")
     }
    }
   } else if ev.kind=="oem" {
    a:=r.feed(ev.inst)
    if a.kind==""||!ready||!enable{continue}
    if a.kind=="arm" || a.kind=="expand"{
     if !c.healthy(){continue}
     _,_ = io.WriteString(stdin,"ARM "+a.id+"\n")
    }
    if a.kind=="expand"{
     if !c.healthy(){continue}
     // User explicitly asked to re-answer the immediate last OEM
     // knowledge question. No automatic handoff for normal answers.
     if !c.submit(a){log.Print("previous-answer expansion was not accepted")}
    }
    if a.kind=="expand_unavailable"{log.Print("no immediately preceding OEM knowledge answer")}
   }
  }
 }
}
func main(){
 var injector=flag.String("injector","/data/adb/xiaonexus/frida-inject","official Frida 16.6.6 Android ARM binary")
 var script=flag.String("script","/data/adb/xiaonexus/lx04_nexus_gate.js","pinned XiaoNexus JavaScript")
 var tokenPath=flag.String("token","/data/adb/xiaonexus/control.token","owner-only receiver token")
 var enable=flag.Bool("enable",false,"enable real refusal PCM gating; default OBSERVE ONLY")
 flag.Parse()
 data,err:=os.ReadFile(*tokenPath)
 if err!=nil{log.Fatal("receiver token absent; fail-open")}
 secret:=strings.TrimSpace(string(data))
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
