package main

import (
 "encoding/json"
 "fmt"
 "strings"
 "testing"
 "time"
)

func makeLog(namespace,name,id string,body interface{})string {
 b,_:=json.Marshal(map[string]interface{}{
 "header":map[string]string{"namespace":namespace,"name":name,"dialog_id":id},
 "payload":body})
 return "MICO.cap instruction="+string(b)
}
func TestParseFinalASRAndKnowledge(t *testing.T){
 id:=strings.Repeat("a",32)
 l:=makeLog("SpeechRecognizer","RecognizeResult",id,
   map[string]interface{}{"is_final":true,"results":[]interface{}{map[string]string{"text":" 什么是黑洞 "}}})
 i,ok:=parseOEM(l);if !ok||i.kind!="asr"||i.text!="什么是黑洞"||i.id!=id{t.Fatal("final ASR parse",i)}
 p:=makeLog("SpeechRecognizer","RecognizeResult",id,
   map[string]interface{}{"is_final":false,"results":[]interface{}{map[string]string{"text":"黑洞"}})
 if _,ok:=parseOEM(p);ok{t.Fatal("partial ASR must not arm")}
 k:=makeLog("Template","General",id,map[string]interface{}{"text":"黑洞是时空区域","skill_icon":map[string]string{"description":"智能问答"}})
 if i,ok:=parseOEM(k);!ok||i.kind!="knowledge"{t.Fatal("knowledge",i)}
}
func TestExpandsLastNativeKnowledgeOnly(t *testing.T){
 a,b,c:=strings.Repeat("a",32),strings.Repeat("b",32),strings.Repeat("c",32)
 now:=time.Unix(1,0)
 r:=newRouter(func()time.Time{return now})
 if x:=r.feed(instruction{"asr",a,"黑洞是什么"});x.kind!="arm"{t.Fatal(x)}
 r.feed(instruction{"knowledge",a,"一种时空区域"})
 now=now.Add(time.Second)
 if x:=r.feed(instruction{"asr",b,"让DeepSeek完善下小爱同学上轮的回答"});x.kind!="expand"||x.query!="黑洞是什么"{t.Fatal(x)}
 if x:=r.gate(b);x.kind!=""{t.Fatal("expand must never route refusal",x)}
 if x:=r.feed(instruction{"asr",c,"让DeepSeek完善下小爱同学上轮的回答"});x.kind!="expand_unavailable"{t.Fatal(x)}
}
func TestOwnerGateAndDedup(t *testing.T){
 id:=strings.Repeat("f",32)
 now:=time.Unix(1,0);r:=newRouter(func()time.Time{return now})
 r.feed(instruction{"asr",id,"为什么海水是咸的"})
 if x:=r.gate(id);x.kind!="refusal"||x.query!="为什么海水是咸的"{t.Fatal(x)}
 if x:=r.gate(id);x.kind!=""{t.Fatal("duplicate")}
}
func TestRejectUnmatchedAndLate(t *testing.T){
 a,b:=strings.Repeat("a",32),strings.Repeat("b",32)
 now:=time.Unix(1,0);r:=newRouter(func()time.Time{return now})
 r.feed(instruction{"asr",a,"计算圆周率"})
 if x:=r.gate(b);x.kind!=""{t.Fatal(x)}
 now=now.Add(13*time.Second)
 if x:=r.gate(a);x.kind!=""{t.Fatal(x)}
}
func TestOnlyExactExpansionPhrases(t *testing.T){
 if !isExpand("让DeepSeek完善下小爱同学上轮的回答"){t.Fatal("expected command")}
 if isExpand("小爱请帮我关灯，再让DeepSeek完善下小爱同学上轮的回答"){t.Fatal("too broad")}
}
func TestParseFridaGateEnvelope(t *testing.T){
 id:=strings.Repeat("c",32)
 line:=fmt.Sprintf(`{"type":"send","payload":{"kind":"gate_accepted","dialog_id":"%s"}}`,id)
 if got,ok:=parseGate(line);!ok||got!=id{t.Fatal(got,ok)}
 if _,ok:=parseGate(`{"type":"send","payload":{"kind":"other","dialog_id":"cccc"}}`);ok{t.Fatal("unexpected gate")}
}

func TestRejectNonKnowledgeGeneral(t *testing.T) {
 id:=strings.Repeat("d",32)
 for _,skill:=range []string{"计算器","音乐","米家",""} {
  payload:=map[string]interface{}{"text":"1+99×2=199","skill_icon":map[string]string{"description":skill}}
  if i,ok:=parseOEM(makeLog("Template","General",id,payload));ok {
   t.Fatalf("nonknowledge skill %q accepted as %v",skill,i)
  }
 }
 if _,ok:=parseOEM(makeLog("Template","General",id,map[string]string{"text":"1+99×2=199"}));ok {
  t.Fatal("missing skill icon must not qualify as knowledge")
 }
}
func TestNonKnowledgeCannotExpand(t *testing.T) {
 a,b:=strings.Repeat("a",32),strings.Repeat("b",32)
 now:=time.Unix(1,0)
 r:=newRouter(func()time.Time{return now})
 r.feed(instruction{"asr",a,"一加九十九乘二等于几"})
 // A calculator card is rejected at the parser, so no native knowledge is stored.
 calc:=makeLog("Template","General",a,map[string]interface{}{"text":"1+99×2=199","skill_icon":map[string]string{"description":"计算器"}})
 if inst,ok:=parseOEM(calc);ok {r.feed(inst);t.Fatal("calculator must not pass parser")}
 if x:=r.feed(instruction{"asr",b,"让DeepSeek完善下小爱同学上轮的回答"});x.kind!="expand_unavailable" {t.Fatal(x)}
}
