// Package starterpolicy contains deterministic, side-effect-free policy selection
// for onboarding starter offers. Persistence and subscription provisioning are
// deliberately owned by the application service.
package starterpolicy

import (
 "errors"
 "sort"
 "strings"
 "time"
)

type Trigger string
const (
 AfterVerification Trigger = "email_verified"
 FirstLogin Trigger = "first_login"
)

type Policy struct {
 ID string
 PlanID string
 Enabled bool
 Priority int
 Region string // empty means all regions
 Trigger Trigger
 ReservationTTL time.Duration
}

type Candidate struct {
 UserID string
 EmailVerified bool
 IsNewUser bool
 Region string
 Trigger Trigger
}

var ErrInvalidPolicy = errors.New("invalid starter policy")

func Validate(p Policy) error {
 if strings.TrimSpace(p.ID)=="" || strings.TrimSpace(p.PlanID)=="" || p.ReservationTTL<=0 { return ErrInvalidPolicy }
 switch p.Trigger {
 case AfterVerification, FirstLogin:
 default: return ErrInvalidPolicy
 }
 return nil
}

// Select returns the highest-priority eligible policy. Ties are resolved by
// policy ID for stable outcomes across application instances. Callers MUST
// enforce one-time claims transactionally in the database; selection alone
// cannot prevent duplicate grants.
func Select(policies []Policy, c Candidate) (Policy, bool) {
 if !c.IsNewUser || strings.TrimSpace(c.UserID)=="" { return Policy{}, false }
 eligible:=make([]Policy,0,len(policies))
 for _,p:=range policies {
  if !p.Enabled || Validate(p)!=nil || p.Trigger!=c.Trigger { continue }
  if !c.EmailVerified { continue } // never grant to unverified users
  if p.Region!="" && !strings.EqualFold(p.Region,c.Region) { continue }
  eligible=append(eligible,p)
 }
 sort.Slice(eligible,func(i,j int)bool{
  if eligible[i].Priority!=eligible[j].Priority { return eligible[i].Priority>eligible[j].Priority }
  return eligible[i].ID<eligible[j].ID
 })
 if len(eligible)==0 { return Policy{},false }
 return eligible[0],true
}
