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
var validID = regexp.MustCompile("^[0-9a-fA-F]{31,32}$")
var validGateNote = regexp.MustCompile(`^XIAONEXUS_GATE_NOTE:[A-Za-z0-9_. :\-]{1,100}$`)
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
func voiceUpdateMode(text string) string {
 switch canon(text) {
 case "检查更新","小纽检查更新":return "check"
 case "立即升级","小纽立即升级":return "apply"
 case "当前版本":return "version"
 case "清理旧版本":return "cleanup"
 default:return ""
 }
}
func directQuery(text string,prefixes []string)string {
 text=strings.TrimSpace(text)
 for _,prefix:=range prefixes{
  if len(prefix)==0 || len(text)<len(prefix) || !strings.EqualFold(text[:len(prefix)],prefix){continue}
  query:=strings.TrimLeftFunc(text[len(prefix):],func(r rune)bool{return unicode.IsSpace(r)||strings.ContainsRune(":：,，",r)})
  if n:=len([]rune(query));n>0&&n<=600{return query}
 }
 return ""
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
 expansions []string
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
  custom := false
  n:=canon(i.text)
  if n!="" && !hasAny(n,nativeActions) {
   for _,phrase:=range r.expansions {
    if n==canon(phrase) { custom=true; break }
   }
  }
  expand=expand||custom
  if custom || (expand && isBriefExpand(i.text)) {
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

type client struct{ token string; http *http.Client }

type receiverStatus struct {
 Rules struct {
  Refusals []string `json:"refusals"`
  Expansions []string `json:"expansions"`
  Direct []string `json:"direct"`
 } `json:"rules"`
 AgentBusy bool `json:"agent_busy"`
 DoorbellBlocked bool `json:"doorbell_blocked"`
 UpdatePending bool `json:"update_pending"`
 UpdateOffer string `json:"update_offer"`
 WaitingDialog string `json:"waiting_dialog_id"`
 Healthy bool `json:"healthy"`
 Ready bool `json:"ready"`
 RoutingBlocked bool `json:"routing_blocked"`
 NoGlobalMute bool `json:"no_global_mute"`
}

func (s receiverStatus) alive() bool {
 return s.Healthy && s.NoGlobalMute
}
func (s receiverStatus) routeReady() bool {
 return s.alive() && s.Ready && !s.RoutingBlocked
}
func (c *client) status(parent context.Context) (receiverStatus, bool) {
 ctx,cancel:=context.WithTimeout(parent,900*time.Millisecond);defer cancel()
 req,err:=http.NewRequestWithContext(ctx,http.MethodGet,receiver+"/v1/health",nil)
 if err!=nil{return receiverStatus{},false}
 req.Header.Set("Authorization","Bearer "+c.token)
 res,err:=c.http.Do(req);if err!=nil{return receiverStatus{},false}
 defer res.Body.Close()
 if res.StatusCode!=http.StatusOK{return receiverStatus{},false}
 var status receiverStatus
 if json.NewDecoder(io.LimitReader(res.Body,16384)).Decode(&status)!=nil{return receiverStatus{},false}
 return status,true
}
func (c *client) waitForReceiver(parent context.Context) (receiverStatus, error) {
 ctx,cancel:=context.WithTimeout(parent,25*time.Second)
 defer cancel()
 ticker:=time.NewTicker(time.Second)
 defer ticker.Stop()
 for {
  status,ok:=c.status(ctx)
  if ok&&status.alive(){return status,nil}
  select {
  case <-ctx.Done():
   if err:=parent.Err();err!=nil{return receiverStatus{},err}
   return receiverStatus{},errors.New("answer receiver unavailable; no injection")
  case <-ticker.C:
  }
 }
}

func (c *client) voiceUpdate(mode, dialog string) {
 ctx,cancel:=context.WithTimeout(context.Background(),900*time.Millisecond);defer cancel()
 body,_:=json.Marshal(mode)
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,receiver+"/v1/update/voice",bytes.NewReader(body))
 if err!=nil{return}
 req.Header.Set("Authorization","Bearer "+c.token)
 req.Header.Set("Content-Type","application/json")
 req.Header.Set("X-XiaoNexus-Dialog",dialog)
 response,err:=c.http.Do(req)
 if err!=nil{return}
 defer response.Body.Close()
 if response.StatusCode==http.StatusNoContent{log.Print("XIAONEXUS_VOICE_UPDATE_ACCEPTED")}else{log.Printf("XIAONEXUS_VOICE_UPDATE_REJECTED status=%d",response.StatusCode)}
}
func(c *client) submit(a action)bool {
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second);defer cancel()
 body:=map[string]string{"kind":a.kind,"dialog_id":a.id,"query":a.query}
 if a.kind=="expand" {body["native_answer"]=a.answer}
 buf,err:=json.Marshal(body);if err!=nil{return false}
 req,err:=http.NewRequestWithContext(ctx,http.MethodPost,receiver+"/v1/query",bytes.NewReader(buf))
 if err!=nil{return false}
 req.Header.Set("Authorization","Bearer "+c.token)
 req.Header.Set("Content-Type","application/json")
 res,err:=c.http.Do(req);if err!=nil{return false}
 defer res.Body.Close()
 return res.StatusCode==http.StatusAccepted
}
func oemPID() (int,error) {
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second);defer cancel()
 out,err:=exec.CommandContext(ctx,"/system/bin/ps","-A").Output()
 if err!=nil{return 0,err}
 return parseOEMProcessList(string(out))
}
func parseOEMProcessList(out string)(int,error) {
 parents:=make(map[int]bool)
 for _,line:=range strings.Split(out,"\n") {
  f:=strings.Fields(line)
  if len(f)>=3 && (f[len(f)-1]=="zygote"||f[len(f)-1]=="zygote64") {
   pid,e:=strconv.Atoi(f[1]);if e==nil&&pid>0{parents[pid]=true}
  }
 }
 found,pid:=0,0
 for _,line:=range strings.Split(out,"\n") {
  f:=strings.Fields(line)
  if len(f)<3||f[len(f)-1]!="com.xiaomi.micolauncher"{continue}
  parent,e:=strconv.Atoi(f[2]);if e!=nil||!parents[parent]{continue}
  n,e:=strconv.Atoi(f[1]);if e!=nil||n<=0{continue}
  found++;pid=n
 }
 if found!=1{return 0,fmt.Errorf("expected one zygote-owned OEM process, found %d",found)}
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
// parseArmResult validates a single machine-generated acknowledgement.
func parseArmResult(line string)(string,bool,bool){
 constPrefix:="XIAONEXUS_ARM:"
 if !strings.HasPrefix(line,constPrefix){return "",false,false}
 body:=strings.TrimPrefix(line,constPrefix)
 parts:=strings.Split(body,":")
 if len(parts)!=2||!validID.MatchString(parts[0])||(parts[1]!="0"&&parts[1]!="1"){return "",false,false}
 return strings.ToLower(parts[0]),parts[1]=="1",true
}
func parseArmReason(line string)(string,bool){
 constPrefix:="XIAONEXUS_ARM_REASON:"
 if !strings.HasPrefix(line,constPrefix){return "",false}
 reason:=strings.TrimPrefix(line,constPrefix)
 for _,c:=range reason {if !(c>='A'&&c<='Z')&&c!='_'&&c!=':'{return "",false}}
 if len(reason)>80||len(reason)<8{return "",false}
 return reason,true
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
 initial,err:=c.waitForReceiver(ctx)
 if err!=nil{return err}
 pid,err:=oemPID();if err!=nil{return err}
 ctx,cancel:=context.WithCancel(ctx);defer cancel()
 inject:=exec.CommandContext(ctx,injector,"-p",strconv.Itoa(pid),"-s",script,"-i")
 stdout,err:=inject.StdoutPipe();if err!=nil{return err}
 stdin,err:=inject.StdinPipe();if err!=nil{return err}
 inject.Stderr=io.Discard
 if err:=inject.Start();err!=nil{return err}
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
 lastActivity:=time.Now()
 directClaims:=make(map[string]time.Time)
 sentDirect:=false
 sentOffer:=""
 updateWaitReason:=""
 slots:=slotControl{root:os.Getenv("XIAONEXUS_SLOT_ROOT"),trial:os.Getenv("XIAONEXUS_TRIAL_SLOT"),deadline:time.Now().Add(45*time.Second)}
 // The JS gate starts BLOCKED. It may only be unblocked after both the
 // hook and the receiver report route readiness.
 waitingDialog:=""
 stopWait:=func(){if waitingDialog!=""{_,_=io.WriteString(stdin,"WAITEND "+waitingDialog+"\n");waitingDialog=""}}
 defer stopWait()
 routingBlocked:=true
 setRoutingBlocked:=func(blocked bool){
  if !enable||blocked==routingBlocked{return}
  command:="UNBLOCK\n"
  if blocked{command="BLOCK\n";stopWait()}
  if _,e:=io.WriteString(stdin,command);e!=nil{
   log.Print("XIAONEXUS_ROUTING_CONTROL_WRITE_FAILED")
   return
  }
  routingBlocked=blocked
  if blocked{log.Print("XIAONEXUS_ROUTING_BLOCKED_PASS_ALL")}else{log.Print("XIAONEXUS_ROUTING_RESUMED")}
 }
 blockNow:=func(){
  if !enable{return}
  if _,e:=io.WriteString(stdin,"BLOCK\n");e!=nil{
   log.Print("XIAONEXUS_ROUTING_CONTROL_WRITE_FAILED")
   return
  }
  routingBlocked=true
  stopWait()
 }
 refreshRoute:=func() (receiverStatus,error){
  status,ok:=c.status(ctx)
  if !ok{status=receiverStatus{}}
  wasTrial:=slots.trial!=""
  if err:=slots.verify(ready,status,time.Now());err!=nil{return status,err}
  if wasTrial && slots.trial==""{log.Print("XIAONEXUS_UPDATE_TRIAL_COMMITTED")}
  if slots.trial!=""||!status.routeReady(){setRoutingBlocked(true);return status,nil}
  setRoutingBlocked(false)
  if !status.AgentBusy {
   rules:=status.Rules.Direct
   if rules==nil{rules=[]string{}}
   prefixes,_:=json.Marshal(rules)
   _,_=io.WriteString(stdin,"DIRECT "+string(prefixes)+"\n")
   sentDirect=true
   if status.UpdateOffer!=sentOffer && validID.MatchString(status.UpdateOffer) {
    if _,err:=io.WriteString(stdin,"UPDATE_OFFER "+status.UpdateOffer+"\n");err==nil{sentOffer=status.UpdateOffer}
   }
  } else if sentDirect {
   _,_=io.WriteString(stdin,"DIRECT null\n")
   sentDirect=false
  }
  if waitingDialog!="" && status.WaitingDialog!=waitingDialog {stopWait()}
  return status,nil
 }
 tryDirect:=func(id string) {
  seen,ok:=directClaims[id]
  if !ok{return}
  d:=r.dialogs[id]
  if d==nil{return}
  delete(directClaims,id)
  if d.routed || time.Since(seen)>3*time.Second || time.Since(d.at)>3*time.Second || routingBlocked{return}
  status,ok:=c.status(ctx)
  if !ok || !status.routeReady() || status.AgentBusy{return}
  if voiceUpdateMode(d.query)!=""{return}
  query:=directQuery(d.query,status.Rules.Direct)
  if query==""{return}
  d.routed=true
  if c.submit(action{kind:"direct",id:id,query:query}){log.Print("XIAONEXUS_DIRECT_ACCEPTED")}
 }
 if enable&&!initial.routeReady(){log.Print("XIAONEXUS_ROUTING_BLOCKED_INITIAL_PASS_ALL")}
 tick:=time.NewTicker(time.Second);defer tick.Stop()
 for {
  select {
  case <-ctx.Done():return nil
  case <-tick.C:
   for id,at:=range directClaims{if time.Since(at)>3*time.Second{delete(directClaims,id)}}
   current,e:=oemPID()
   if e!=nil||current!=pid{return errors.New("OEM PID changed; detach and fail-open")}
   if enable&&(ready||slots.trial!=""){
    status,err:=refreshRoute()
    if err!=nil{return err}
    if slots.root!="" && slots.trial=="" && ready && status.alive() && status.UpdatePending {
     var reason string
     switch {
     case status.DoorbellBlocked:reason="doorbell"
     case status.AgentBusy || waitingDialog!="":reason="answer-busy"
     case time.Since(lastActivity)<60*time.Second || len(events)!=0:reason="recent-voice-activity"
     default:reason=androidAudioState(ctx)
     }
     if reason=="idle" {
      current,ok:=c.status(ctx)
      if ok && current.alive() && current.UpdatePending && !current.AgentBusy && !current.DoorbellBlocked && len(events)==0 {
       blockNow()
       return errSwitchSlot
      }
      reason="receiver-state-changed"
     }
     if reason!=updateWaitReason {
      log.Printf("XIAONEXUS_UPDATE_WAIT reason=%s",reason)
      updateWaitReason=reason
     }
    } else {updateWaitReason=""}
   }
  case ev:=<-events:
   if ev.kind=="fatal"{return errors.New(ev.value)}
   if ev.kind=="inject"{
    if ev.value=="XIAONEXUS_RECOGNIZE_ACTIVITY"{lastActivity=time.Now();continue}
    if command,ok:=strings.CutPrefix(ev.value,"XIAONEXUS_MANAGEMENT:");ok {
     mode,id,ok:=strings.Cut(command,":")
     if ok && enable && ready && !routingBlocked && validID.MatchString(id) {
      switch mode {
      case "check","apply","version":c.voiceUpdate(mode,id)
      case "cleanup":if slots.trial=="" {c.voiceUpdate(mode,id)}
      }
     }
     continue
    }
    if id,ok:=strings.CutPrefix(ev.value,"XIAONEXUS_UPDATE_CONFIRM:");ok {
     if enable && ready && !routingBlocked && id==sentOffer && validID.MatchString(id) {
      c.voiceUpdate("confirm",id)
     }
     continue
    }
    if id,ok:=strings.CutPrefix(ev.value,"XIAONEXUS_DIRECT:");ok && enable && ready && validID.MatchString(id){
     if len(directClaims)<16{
      id=strings.ToLower(id)
      directClaims[id]=time.Now()
      tryDirect(id)
     }
     continue
    }
    if strings.HasPrefix(ev.value,"XIAONEXUS_GATE_NOTE:") && validGateNote.MatchString(ev.value) {log.Print(ev.value);continue}
    if strings.HasPrefix(ev.value,"XIAONEXUS_NATIVE_WAIT_") && len(ev.value)<120 {log.Print(ev.value);continue}
    if id,accepted,ok:=parseArmResult(ev.value);ok {
     mode:="REFUSAL";if d:=r.dialogs[id];d!=nil&&d.expand{mode="EXPAND"}
     state:="REJECTED";if accepted{state="ACCEPTED"}
     log.Printf("XIAONEXUS_ARM_%s_%s dialog8=%s",mode,state,id[:8]);continue
    }
    if reason,ok:=parseArmReason(ev.value);ok{log.Print("XIAONEXUS_ARM_REASON:",reason);continue}
    if ev.value=="XIAONEXUS_READY"{
     ready=true
     log.Print("XiaoNexus injector READY_PASS_ALL")
     if enable{if _,err:=refreshRoute();err!=nil{return err}}
     continue
    }
    if strings.Contains(ev.value,`"type":"error"`){return errors.New("Frida JavaScript exception")}
    if id,ok:=parseGate(ev.value);ok{
     if !enable{continue}
     status,statusOK:=c.status(ctx)
     if routingBlocked||!statusOK||!status.routeReady(){
      blockNow()
      log.Print("XIAONEXUS_GATE_BLOCKED_PASS_ALL")
      continue
     }
     a:=r.gate(id)
     if a.kind=="" {
      if d:=r.dialogs[id];d!=nil&&d.expand&&r.clock().Sub(d.at)>12*time.Second{
       log.Print("XIAONEXUS_EXPAND_GATE_TIMEOUT")
      }else{log.Print("XIAONEXUS_GATE_UNMATCHED")}
      blockNow()
      continue
     }
     if a.kind=="expand"{log.Print("XIAONEXUS_EXPAND_GATE_ACCEPTED")}
     if (a.kind=="refusal"||a.kind=="expand")&&!c.submit(a){
      blockNow()
      log.Print("XIAONEXUS_RECEIVER_SUBMIT_FAILED_PASS_ALL")
      continue
     }
     if a.kind=="refusal"||a.kind=="expand" {
      stopWait();waitingDialog=a.id
      _,_ = io.WriteString(stdin,"WAITBEGIN "+a.id+"\n")
     }
    }
   } else if ev.kind=="oem" {
    var status receiverStatus
    if ev.inst.kind=="asr" {lastActivity=time.Now()}
    managementASR:=ev.inst.kind=="asr" && voiceUpdateMode(ev.inst.text)!=""
    if ev.inst.kind=="asr" && !managementASR && ready && enable && !routingBlocked {
     current,ok:=c.status(ctx)
     if !ok||!current.routeReady(){blockNow()}else{status=current}
    }
    r.expansions=status.Rules.Expansions
    if ev.inst.kind=="asr" {log.Printf("XIAONEXUS_FINAL_ASR_SEEN dialog8=%s",ev.inst.id[:8])}
    a:=r.feed(ev.inst)
    if managementASR{continue}
    if ev.inst.kind=="knowledge"&&r.previous!=nil&&r.previous.dialogID==ev.inst.id{
     log.Printf("XIAONEXUS_EXPAND_KNOWLEDGE_SAVED dialog8=%s",ev.inst.id[:8])
    }
    if a.kind==""||!ready||!enable{continue}
    if ev.inst.kind=="asr" {
     if canon(ev.inst.text)=="测试续听" {continue}
     if directQuery(ev.inst.text,status.Rules.Direct)!="" {
      log.Print("XIAONEXUS_DIRECT_PREFIX_SEEN")
      tryDirect(ev.inst.id)
      continue
     }
    }
    if a.kind=="arm"||a.kind=="arm_expand"{
     if a.kind=="arm_expand"{log.Printf("XIAONEXUS_EXPAND_ARM_REQUESTED dialog8=%s",a.id[:8])}
     if routingBlocked{continue}
     if !status.routeReady(){blockNow();continue}
     refusals,_:=json.Marshal(status.Rules.Refusals)
     if _,e:=io.WriteString(stdin,"REFUSALS "+string(refusals)+"\n");e!=nil{
      blockNow()
      continue
     }
     command:="ARM "
     if a.kind=="arm_expand"{command="ARMEXP "}
     if _,e:=io.WriteString(stdin,command+a.id+"\n");e!=nil{
      blockNow()
      log.Print("XIAONEXUS_ARM_WRITE_FAILED_PASS_ALL")
     }
    }
    if a.kind=="expand_unavailable"{log.Print("XIAONEXUS_EXPAND_CONTEXT_MISSING")}
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
 time.Local = time.FixedZone("CST", 8*60*60)
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
 if err=run(ctx,*injector,*script,*enable,c);errors.Is(err,errSwitchSlot){os.Exit(75)}else if err!=nil{log.Fatal("XiaoNexus stopped (fail-open): ",err)}
}
