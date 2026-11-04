// SPDX-License-Identifier: MIT
// Cached-credential renewal for the pinned MiService client. No password fallback.
package miservice
import("context";"errors";"net/http")
type cachedContextTransport struct{ctx context.Context;base http.RoundTripper}
func(t cachedContextTransport)RoundTrip(r *http.Request)(*http.Response,error){return t.base.RoundTrip(r.WithContext(t.ctx))}
func(a *Account) RenewCachedSID(ctx context.Context,sid string) error {
 if err:=ctx.Err();err!=nil{return err}
 local:=*a
 if local.token==nil{if err:=local.LoadCachedSID(sid);err!=nil{return err}}
 if local.token.UserName!=local.username||local.token.PassToken==""{return errors.New("verified cached passToken required; no password login")}
 snapshot:=*local.token;snapshot.Sids=make(map[string]SidToken)
 for k,v:=range local.token.Sids{snapshot.Sids[k]=v};local.token=&snapshot
 client:=*a.client;transport:=client.Transport;if transport==nil{transport=http.DefaultTransport}
 client.Transport=cachedContextTransport{ctx,transport};local.client=&client
 if err:=local.login(sid,false);err!=nil{return err}
 a.token=local.token
 return nil
}
