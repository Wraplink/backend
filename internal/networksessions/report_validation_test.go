package networksessions

import (
 "testing"
 "time"

 "github.com/google/uuid"
)

func TestReportIdentityRequiresServerResolvedSNIOwner(t *testing.T) {
 user:=uuid.New()
 subscription:=uuid.New()
 cases:=[]struct{
  name,mode string
  userID,subscriptionID uuid.UUID
  want bool
 }{
  {"sni omitted","sni",uuid.Nil,uuid.Nil,true},
  {"sni asserted owner","sni",user,subscription,false},
  {"sni partial owner","sni",user,uuid.Nil,false},
  {"route complete","route",user,subscription,true},
  {"route omitted","route",uuid.Nil,uuid.Nil,false},
  {"route partial","route",user,uuid.Nil,false},
 }
 for _,tc:=range cases {
  t.Run(tc.name,func(t *testing.T){
   if got:=validReportIdentity(tc.mode,tc.userID,tc.subscriptionID);got!=tc.want {
    t.Fatalf("validReportIdentity(%q)=%v, want %v",tc.mode,got,tc.want)
   }
  })
 }
}

func TestSessionReportTimestampBounds(t *testing.T) {
 now:=time.Now().UTC().Truncate(time.Second)
 base:=Session{
  AgentID:"agent-1",SessionID:"session-1",
  UserID:"user-1",SubscriptionID:"subscription-1",
  ClientIP:"192.0.2.10",Mode:"sni",Protocol:"tcp",
  ConnectedAt:now.Add(-time.Minute),LastActivityAt:now,
 }
 cases:=[]struct{
  name string
  change func(*Session)
  valid bool
 }{
  {"valid",func(*Session){},true},
  {"future connection",func(s *Session){s.ConnectedAt=now.Add(time.Second);s.LastActivityAt=s.ConnectedAt},false},
  {"future activity",func(s *Session){s.LastActivityAt=now.Add(time.Second)},false},
  {"activity before connection",func(s *Session){s.LastActivityAt=s.ConnectedAt.Add(-time.Second)},false},
  {"missing connection",func(s *Session){s.ConnectedAt=time.Time{}},false},
  {"missing activity",func(s *Session){s.LastActivityAt=time.Time{}},false},
  {"sni udp",func(s *Session){s.Protocol="udp"},false},
 }
 for _,tc:=range cases {
  t.Run(tc.name,func(t *testing.T){
   s:=base
   tc.change(&s)
   err:=validate(s,now)
   if (err==nil)!=tc.valid {t.Fatalf("validate error=%v, want valid=%v",err,tc.valid)}
  })
 }
}
