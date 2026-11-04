package main
import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"
)
func TestRequestValidation(t *testing.T) {
 id:=strings.Repeat("a",32)
 for _,tc:=range []struct{r request;good bool}{
  {request{Kind:"refusal",DialogID:id,Query:"什么是光"},true},
  {request{Kind:"expand",DialogID:id,Query:"什么是光",NativeAnswer:"光是电磁波"},true},
  {request{Kind:"expand",DialogID:id,Query:"什么是光"},false},
  {request{Kind:"refusal",DialogID:id,Query:"什么是光",NativeAnswer:"光是电磁波"},false},
  {request{Kind:"native",DialogID:id,Query:"关灯"},false},
  {request{Kind:"refusal",DialogID:"bad",Query:"问答"},false},
  {request{Kind:"refusal",DialogID:id,Query:""},false},
 } {
  if got:=tc.r.validate()==nil;got!=tc.good {t.Fatalf("validate=%v want=%v kind=%s",got,tc.good,tc.r.Kind)}
 }
}
func TestExpandOriginalQuestion(t *testing.T) {
 q:=answerQuery(request{Kind:"expand",Query:"介绍太阳系",NativeAnswer:"有八颗行星"})
 if !strings.Contains(q,"原始问题：介绍太阳系") || !strings.Contains(q,"可能过于简略或有误") {t.Fatal("must answer original question rather than blindly expand")}
}
func TestToken(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"private","control.token")
 a,err:=token(path)
 if err!=nil || len(a)!=64 {t.Fatal(err)}
 b,err:=token(path)
 if err!=nil || a!=b {t.Fatal("token must persist")}
 if err:=os.WriteFile(path,[]byte("bad"),0600);err!=nil {t.Fatal(err)}
 if _,err:=token(path);err==nil {t.Fatal("invalid token accepted")}
}

func TestWaitGenerationIsolation(t *testing.T) {
 s := &server{generation:2, waitingDialog:strings.Repeat("b",32)}
 s.finishWaiting(1)
 if s.waitingDialog=="" {t.Fatal("stale completion cleared newer wait")}
 s.finishWaiting(2)
 if s.waitingDialog!="" {t.Fatal("completion left wait active")}
 s.waitingDialog=strings.Repeat("c",32)
 s.preemptForDoorbell()
 if s.waitingDialog!="" || s.generation!=3 {t.Fatal("doorbell did not clear current generation")}
}

type fakeSpeech struct {calls,refresh int;err error;refreshErr error; succeedAfterRefresh bool;onRefresh func()}
func(f *fakeSpeech) MiTTSContext(context.Context,string)error {f.calls++;if f.succeedAfterRefresh&&f.refresh>0{return nil};return f.err}
func(f *fakeSpeech) RefreshMIoTCachedSID(context.Context)error {f.refresh++;if f.onRefresh!=nil{f.onRefresh()};return f.refreshErr}
func TestTTSAuthRecoveryIsBounded(t *testing.T){
 f:=&fakeSpeech{err:errors.New("MIoT RC4 authentication rejected (HTTP 401; no automatic login or retry)"),succeedAfterRefresh:true};s:=&server{box:f}
 if e:=s.nativeSpeech(context.Background(),"test");e!=nil||f.calls!=2||f.refresh!=1{t.Fatal("401 recovery failed",e,f.calls,f.refresh)}
 f.succeedAfterRefresh=false
 if e:=s.nativeSpeech(context.Background(),"test");e==nil||f.refresh!=1{t.Fatal("401 refresh cooldown bypassed")}
}
func TestTTSDoesNotRefresh429OrTransportFailures(t *testing.T){
 for _,msg:=range []string{"MIoT RC4 HTTP 429","MIoT RC4 access forbidden (HTTP 403)","connection timeout"}{
  f:=&fakeSpeech{err:errors.New(msg)};s:=&server{box:f}
  if e:=s.nativeSpeech(context.Background(),"test");e==nil||f.calls!=1||f.refresh!=0{t.Fatal("unsafe retry",msg)}
 }
}
func TestTTSRefreshFailureAndCancelNeverReplay(t *testing.T){
 f:=&fakeSpeech{err:errors.New("HTTP 401; rejected"),refreshErr:errors.New("rate limited")};s:=&server{box:f}
 if e:=s.nativeSpeech(context.Background(),"test");e==nil||f.calls!=1||f.refresh!=1{t.Fatal("refresh failure replayed")}
 ctx,cancel:=context.WithCancel(context.Background());cancel();f=&fakeSpeech{};s=&server{box:f}
 if e:=s.nativeSpeech(ctx,"test");e!=context.Canceled||f.calls!=0{t.Fatal("cancel sent TTS")}
 ctx,cancel=context.WithCancel(context.Background());defer cancel();f=&fakeSpeech{err:errors.New("HTTP 401; rejected"),onRefresh:cancel};s=&server{box:f}
 if e:=s.nativeSpeech(ctx,"test");e!=context.Canceled||f.calls!=1{t.Fatal("cancel during refresh replayed")}
}

func Test31DialogValidationPreservesExactIdentity(t *testing.T){
 id:=strings.Repeat("a",31)
 if (request{Kind:"refusal",DialogID:id,Query:"test"}).validate()!=nil{t.Fatal("observed 31-char dialog rejected")}
 for _,bad:=range []string{strings.Repeat("a",30),strings.Repeat("a",33),strings.Repeat("g",31)}{if (request{Kind:"refusal",DialogID:bad,Query:"test"}).validate()==nil{t.Fatal("invalid dialog accepted")}}
}
