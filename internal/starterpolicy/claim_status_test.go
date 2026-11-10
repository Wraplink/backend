package starterpolicy

import (
 "testing"
 "time"
)

func TestDisplayClaimState(t *testing.T) {
 now:=time.Date(2026,11,10,10,0,0,0,time.UTC)
 past:=now.Add(-time.Minute)
 future:=now.Add(time.Minute)
 earlier:=now.Add(-time.Hour)
 active:="active"
 cancelled:="cancelled"
 cases:=[]struct{
  name,state string
  reservation,starts,expires *time.Time
  subscription *string
  want string
 }{
  {"valid reservation","reserved",&future,nil,nil,nil,"reserved"},
  {"expired reservation","reserved",&past,nil,nil,nil,"expired"},
  {"missing reservation deadline","reserved",nil,nil,nil,nil,"expired"},
  {"valid active trial","active",nil,&earlier,&future,&active,"active"},
  {"expired trial","active",nil,&earlier,&past,&active,"expired"},
  {"missing subscription","active",nil,&earlier,&future,nil,"cancelled"},
  {"cancelled subscription","active",nil,&earlier,&future,&cancelled,"cancelled"},
  {"missing start","active",nil,nil,&future,&active,"failed"},
  {"missing expiry","active",nil,&earlier,nil,&active,"failed"},
  {"future start","active",nil,&future,&future,&active,"failed"},
  {"expiry before start","active",nil,&past,&earlier,&active,"failed"},
  {"cancelled claim","cancelled",nil,&earlier,&future,&active,"cancelled"},
  {"failed claim","failed",nil,nil,nil,nil,"failed"},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   got:=DisplayClaimState(tc.state,tc.reservation,tc.starts,tc.expires,tc.subscription,now)
   if got!=tc.want {t.Fatalf("got %q, want %q",got,tc.want)}
  })
 }
}
