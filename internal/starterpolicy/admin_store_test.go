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
  ActivationTrigger:"manual",ReservationTTL:7*24*time.Hour,
 }
 if err:=validateAdminPolicy(p);err!=nil {t.Fatalf("valid policy rejected: %v",err)}
 p.AssignmentTrigger="registration"
 if err:=validateAdminPolicy(p);!errors.Is(err,ErrInvalidPolicy) {t.Fatalf("registration trigger accepted: %v",err)}
 p.AssignmentTrigger=AfterVerification
 p.ActivationTrigger="unknown"
 if err:=validateAdminPolicy(p);!errors.Is(err,ErrInvalidPolicy) {t.Fatalf("unknown activation trigger accepted: %v",err)}
 p.ActivationTrigger="manual"
 p.ReservationTTL=500*time.Millisecond
 if err:=validateAdminPolicy(p);!errors.Is(err,ErrInvalidPolicy) {t.Fatalf("subsecond TTL accepted: %v",err)}
 p.ReservationTTL=time.Hour
 p.PlanID=uuid.Nil
 if err:=validateAdminPolicy(p);!errors.Is(err,ErrInvalidPolicy) {t.Fatalf("missing plan accepted: %v",err)}
}
