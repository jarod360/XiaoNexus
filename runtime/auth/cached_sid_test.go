// SPDX-License-Identifier: MIT
package miservice
import("context";"io";"net/http";"strings";"testing")
type renewTransport func(*http.Request)(*http.Response,error)
func(f renewTransport)RoundTrip(r *http.Request)(*http.Response,error){return f(r)}
func cachedAccount() *Account {a:=NewAccount("speaker-account","must-never-be-used",nil);a.token=NewTokens();a.token.UserName="speaker-account";a.token.PassToken="old-pass";a.token.Sids[MIIO_SID]=SidToken{Ssecurity:"b2xk",ServiceToken:"old-sid"};return a}
func TestCachedRenewRejectsWithoutPasswordFallback(t *testing.T){
 for _,body:=range []string{`{"code":70022}`,`{"code":-500}`} {
  a:=cachedAccount();calls:=0;before:=a.token
  a.client.Transport=renewTransport(func(r *http.Request)(*http.Response,error){calls++;if r.URL.Path!="/pass/serviceLogin"{t.Fatal("password login attempted")};return &http.Response{StatusCode:200,Header:http.Header{},Body:io.NopCloser(strings.NewReader(body)),Request:r},nil})
  if e:=a.RenewCachedSID(context.Background(),MIIO_SID);e==nil{t.Fatal("rejection accepted")}
  if calls!=1||a.token!=before||a.token.PassToken!="old-pass"{t.Fatal("identity changed or repeated login")}
 }
}
func TestCachedRenewRequiresExistingPassAndContext(t *testing.T){
 a:=cachedAccount();a.token.PassToken="";calls:=0
 a.client.Transport=renewTransport(func(r *http.Request)(*http.Response,error){calls++;return nil,nil})
 if e:=a.RenewCachedSID(context.Background(),MIIO_SID);e==nil||calls!=0{t.Fatal("missing cached pass used network")}
 a.token.PassToken="old";ctx,cancel:=context.WithCancel(context.Background());cancel()
 if e:=a.RenewCachedSID(ctx,MIIO_SID);e!=context.Canceled||calls!=0{t.Fatal("cancel ignored")}
}
func TestCachedRenewPreservesOtherSID(t *testing.T){
 a:=cachedAccount();a.token.Sids["micoapi"]=SidToken{Ssecurity:"keep",ServiceToken:"keep-sid"};calls:=0
 a.client.Transport=renewTransport(func(r *http.Request)(*http.Response,error){
  calls++;h:=http.Header{};body:=`{"code":0,"userId":1,"cUserId":"unit","ssecurity":"bmV3","location":"https://unit.invalid/ok?x=1","nonce":1}`
  if calls==2{h.Add("Set-Cookie","serviceToken=new-sid; Path=/");body="{}"}
  return &http.Response{StatusCode:200,Header:h,Body:io.NopCloser(strings.NewReader(body)),Request:r},nil
 })
 if e:=a.RenewCachedSID(context.Background(),MIIO_SID);e!=nil{t.Fatal(e)}
 if calls!=2||a.token.Sids[MIIO_SID].ServiceToken!="new-sid"||a.token.Sids["micoapi"].ServiceToken!="keep-sid"||a.token.PassToken!="old-pass"{t.Fatal("SID identity not preserved")}
}
