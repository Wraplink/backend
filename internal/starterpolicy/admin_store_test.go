package starterpolicy

import (
 "errors"
 "testing"
 "time"

 "github.com/google/uuid"
)

func TestValidateAdminPolicy(t *testing.T) {
 p:=AdminPolicy{
  PlanID:uuid.New(),AssignmentTrigger:AfterVerification,
  ActivationTrigger:"manual",ReservationTTL:7*24*time.Hour,TrialDuration:time.Hour,
 }
 if err:=validateAdminPolicy(p);err!=nil {t.Fatalf("valid policy rejected: %v",err)}
 p.AssignmentTrigger="registration"
 if err:=validateAdminPolicy(p);!errors.Is(err,ErrInvalidPolicy) {t.Fatalf("registration trigger accepted: %v",err)}
 p.AssignmentTrigger=FirstLogin
 if err:=validateAdminPolicy(p);!errors.Is(err,ErrInvalidPolicy) {t.Fatalf("unimplemented first-login trigger accepted: %v",err)}
 p.AssignmentTrigger=AfterVerification
 p.ActivationTrigger="first_dns_use"
 if err:=validateAdminPolicy(p);!errors.Is(err,ErrInvalidPolicy) {t.Fatalf("unimplemented DNS-use trigger accepted: %v",err)}
 p.ActivationTrigger="immediate"
 if err:=validateAdminPolicy(p);!errors.Is(err,ErrInvalidPolicy) {t.Fatalf("unimplemented immediate trigger accepted: %v",err)}
 p.ActivationTrigger="unknown"
 if err:=validateAdminPolicy(p);!errors.Is(err,ErrInvalidPolicy) {t.Fatalf("unknown activation trigger accepted: %v",err)}
 p.ActivationTrigger="manual"
 p.Region="eu-1"
 if err:=validateAdminPolicy(p);!errors.Is(err,ErrInvalidPolicy) {t.Fatalf("unimplemented region policy accepted: %v",err)}
 p.Region=""
 p.ReservationTTL=500*time.Millisecond
 if err:=validateAdminPolicy(p);!errors.Is(err,ErrInvalidPolicy) {t.Fatalf("subsecond TTL accepted: %v",err)}
 p.ReservationTTL=time.Hour
 p.PlanID=uuid.Nil
 if err:=validateAdminPolicy(p);!errors.Is(err,ErrInvalidPolicy) {t.Fatalf("missing plan accepted: %v",err)}
}

func TestValidateAdminPolicyDurationBoundaries(t *testing.T) {
 base:=AdminPolicy{
  PlanID:uuid.New(),AssignmentTrigger:AfterVerification,
  ActivationTrigger:"manual",ReservationTTL:7*24*time.Hour,TrialDuration:time.Hour,
 }
 cases:=[]struct{
  name string
  ttl time.Duration
  duration time.Duration
  valid bool
 }{
  {"one second trial",time.Second,time.Second,true},
  {"one hour trial",7*24*time.Hour,time.Hour,true},
  {"zero reservation TTL",0,time.Hour,false},
  {"negative reservation TTL",-time.Second,time.Hour,false},
  {"subsecond reservation TTL",time.Millisecond,time.Hour,false},
  {"zero trial duration",time.Hour,0,false},
  {"negative trial duration",time.Hour,-time.Second,false},
  {"subsecond trial duration",time.Hour,time.Millisecond,false},
 }
 for _,tc:=range cases {
  t.Run(tc.name,func(t *testing.T){
   p:=base
   p.ReservationTTL=tc.ttl
   p.TrialDuration=tc.duration
   err:=validateAdminPolicy(p)
   if tc.valid && err!=nil {t.Fatalf("valid duration rejected: %v",err)}
   if !tc.valid && !errors.Is(err,ErrInvalidPolicy) {t.Fatalf("invalid duration accepted: %v",err)}
  })
 }
}
