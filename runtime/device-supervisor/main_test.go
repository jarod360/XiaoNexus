package main

import (
 "encoding/json"
 "fmt"
 "path/filepath"
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
   map[string]interface{}{"is_final":false,"results":[]interface{}{map[string]string{"text":"黑洞"}}})
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
 if x:=r.feed(instruction{"asr",b,"让DeepSeek完善下小爱同学上轮的回答"});x.kind!="arm_expand"{t.Fatal(x)}
 if x:=r.gate(b);x.kind!="expand"||x.query!="黑洞是什么"||x.answer!="一种时空区域"{t.Fatal("expand gate not correlated",x)}
 if x:=r.gate(b);x.kind!=""{t.Fatal("duplicate expansion gate",x)}
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

func TestSupervisorSingletonLock(t *testing.T){
 path:=filepath.Join(t.TempDir(),"supervisor.lock")
 a,e:=singleton(path);if e!=nil{t.Fatal(e)}
 if b,e:=singleton(path);e==nil{b.Close();a.Close();t.Fatal("second supervisor was admitted")}
 a.Close()
 c,e:=singleton(path);if e!=nil{t.Fatal("kernel lock not released after close",e)}
 c.Close()
}

// Same positive/negative corpus as Python's intent tests. We avoid matching
// any arbitrary sentence that merely contains "补充" or "完善".
func TestContextualExpansionVocabulary(t *testing.T){
 positive:=[]string{
  "让DeepSeek完善下小爱同学上轮的回答",
  "补充一下上一轮的问答","完善下刚才的答案","把上一个问题展开讲讲",
  "把刚刚的回答说得更详细些","再详细解释一下之前那个问题",
  "刚才你说的答案，请重新回答","补全上轮回答","纠正一下刚才的解释",
  "展开上个问题的回答",
 }
 for _,s:=range positive {if !isExpand(s){t.Errorf("did not recognize: %s",s)}}
 negative:=[]string{
  "帮我补充水","完善空调设置","详细说一下天气","播放上轮音乐",
  "打开灯再完善设置","补充电池","为什么要补充维生素",
  "今天让DeepSeek完善一下小爱同学上一轮的回答怎么样",
  "关闭门铃","这一轮的答案","讲个笑话",
 }
 for _,s:=range negative {if isExpand(s){t.Errorf("false positive: %s",s)}}
}
func TestBriefExpansionRequiresRecentKnowledge(t *testing.T){
 a,b,c:=strings.Repeat("a",32),strings.Repeat("b",32),strings.Repeat("c",32)
 now:=time.Unix(500,0)
 r:=newRouter(func()time.Time{return now})
 if !isExpand("补充下")||!isExpand("再详细点")||!isExpand("展开讲讲"){t.Fatal("brief intent missing")}
 r.feed(instruction{"asr",a,"什么是黑洞"})
 r.feed(instruction{"knowledge",a,"时空区域"})
 now=now.Add(20*time.Second)
 if x:=r.feed(instruction{"asr",b,"补充下"});x.kind!="arm_expand"{t.Fatal(x)}
 if x:=r.gate(b);x.kind!="expand"||x.query!="什么是黑洞"{t.Fatal(x)}
 z:=newRouter(func()time.Time{return now})
 z.feed(instruction{"asr",a,"什么是地球"})
 z.feed(instruction{"knowledge",a,"一颗行星"})
 now=now.Add(61*time.Second)
 if x:=z.feed(instruction{"asr",c,"补充下"});x.kind=="arm_expand"{t.Fatal("stale brief command triggered")}
}

func TestArmDiagnosticEnvelope(t *testing.T){
 id:=strings.Repeat("a",32)
 if got,accepted,ok:=parseArmResult("XIAONEXUS_ARM:"+id+":1");!ok||!accepted||got!=id {t.Fatalf("accepted: %s %v %v",got,accepted,ok)}
 if got,accepted,ok:=parseArmResult("XIAONEXUS_ARM:"+id+":0");!ok||accepted||got!=id {t.Fatalf("rejected: %s %v %v",got,accepted,ok)}
 for _,invalid:=range []string{"XIAONEXUS_ARM:short:1","XIAONEXUS_ARM:"+id+":2","XIAONEXUS_ARM:"+id+":1 extra"} {
  if _,_,ok:=parseArmResult(invalid);ok{t.Fatalf("accepted invalid message: %q",invalid)}
 }
 if reason,ok:=parseArmReason("XIAONEXUS_ARM_REASON:EXPAND:OWNER_BUSY");!ok||reason!="EXPAND:OWNER_BUSY"{t.Fatal(reason,ok)}
 for _,invalid:=range []string{"XIAONEXUS_ARM_REASON:EXPAND:OWNER_BUSY some voice","XIAONEXUS_ARM_REASON:EX:小爱"} {
  if _,ok:=parseArmReason(invalid);ok{t.Fatalf("leaked unexpected diagnostic: %q",invalid)}
 }
}
