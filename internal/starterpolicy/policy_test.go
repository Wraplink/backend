package starterpolicy

import (
 "testing"
 "time"
)

func TestSelect(t *testing.T) {
 policies:=[]Policy{
  {ID:"b",PlanID:"plan-b",Enabled:true,Priority:10,Trigger:AfterVerification,ReservationTTL:7*24*time.Hour},
  {ID:"a",PlanID:"plan-a",Enabled:true,Priority:10,Trigger:AfterVerification,ReservationTTL:7*24*time.Hour},
  {ID:"high",PlanID:"plan-h",Enabled:true,Priority:20,Region:"eu",Trigger:AfterVerification,ReservationTTL:time.Hour},
 }
 base:=Candidate{UserID:"u",EmailVerified:true,IsNewUser:true,Region:"us",Trigger:AfterVerification}
 p,ok:=Select(policies,base)
 if !ok || p.ID!="a" {t.Fatalf("expected stable tie-break a; got %+v %v",p,ok)}
 base.Region="EU"
 p,ok=Select(policies,base)
 if !ok || p.ID!="high" {t.Fatalf("expected regional priority; got %+v %v",p,ok)}
 base.EmailVerified=false
 if _,ok=Select(policies,base);ok {t.Fatal("unverified user received offer")}
 base.EmailVerified=true
 base.IsNewUser=false
 if _,ok=Select(policies,base);ok {t.Fatal("existing user received onboarding offer")}
 base.IsNewUser=true
 base.Trigger=FirstLogin
 if _,ok=Select(policies,base);ok {t.Fatal("unexpected trigger match")}
}
func TestInvalidPolicy(t *testing.T) {
 if Validate(Policy{ID:"p",PlanID:"x",Trigger:AfterVerification})==nil {t.Fatal("missing TTL accepted")}
 if Validate(Policy{ID:"p",PlanID:"x",Trigger:"bad",ReservationTTL:time.Hour})==nil {t.Fatal("bad trigger accepted")}
}
