package main
import (
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
