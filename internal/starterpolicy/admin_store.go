package starterpolicy

import (
 "context"
 "errors"
 "strings"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)

// AdminPolicy is the persisted configuration used by the admin policy editor.
// HTTP authentication and authorization must be enforced by the existing
// backend admin middleware before calling these methods.
type AdminPolicy struct {
 ID uuid.UUID `json:"id"`
 PlanID uuid.UUID `json:"planId"`
 Enabled bool `json:"enabled"`
 Priority int `json:"priority"`
 Region string `json:"region"`
 AssignmentTrigger Trigger `json:"assignmentTrigger"`
 ActivationTrigger string `json:"activationTrigger"`
 ReservationTTL time.Duration `json:"-"`
 TrialDuration time.Duration `json:"-"`
}

type PolicyStore struct { DB *pgxpool.Pool }

func validateAdminPolicy(p AdminPolicy) error {
 if p.PlanID==uuid.Nil || p.ReservationTTL<=0 || p.ReservationTTL%time.Second!=0 || p.TrialDuration<=0 || p.TrialDuration%time.Second!=0 {
  return ErrInvalidPolicy
 }
 // Only email verification and manual activation are wired end-to-end.
 // Reject other triggers instead of silently creating policies that never run.
 if p.AssignmentTrigger!=AfterVerification || p.ActivationTrigger!="manual" || strings.TrimSpace(p.Region)!="" {
  return ErrInvalidPolicy
 }
 return nil
}

// Create checks free, active and published plan eligibility atomically.
func (s PolicyStore) Create(ctx context.Context,p AdminPolicy) (AdminPolicy,error) {
 if s.DB==nil {return AdminPolicy{},errors.New("starter policy store has no database")}
 if err:=validateAdminPolicy(p);err!=nil{return AdminPolicy{},err}
 if p.ID==uuid.Nil {p.ID=uuid.New()}
 err:=s.DB.QueryRow(ctx,`INSERT INTO starter_plan_policies
 (id,plan_id,enabled,priority,region,assignment_trigger,activation_trigger,reservation_ttl_seconds,trial_duration_seconds)
 SELECT $1,catalog.id,$3,$4,$5,$6,$7,$8,$9 FROM plans catalog
 WHERE catalog.id=$2 AND catalog.price_minor=0 AND catalog.active=true
 AND catalog.archived_at IS NULL
 AND (catalog.publish_at IS NULL OR catalog.publish_at<=now())
 RETURNING id`,p.ID,p.PlanID,p.Enabled,p.Priority,p.Region,p.AssignmentTrigger,p.ActivationTrigger,int64(p.ReservationTTL/time.Second),int64(p.TrialDuration/time.Second)).Scan(&p.ID)
 if errors.Is(err,pgx.ErrNoRows){return AdminPolicy{},ErrNotEligible}
 return p,err
}

// List returns policy configuration in deterministic selection order.
func (s PolicyStore) List(ctx context.Context) ([]AdminPolicy,error) {
 if s.DB==nil {return nil,errors.New("starter policy store has no database")}
 rows,err:=s.DB.Query(ctx,`SELECT id,plan_id,enabled,priority,region,
 assignment_trigger,activation_trigger,reservation_ttl_seconds,trial_duration_seconds
 FROM starter_plan_policies ORDER BY priority DESC,id ASC`)
 if err!=nil{return nil,err}
 defer rows.Close()
 result:=make([]AdminPolicy,0)
 for rows.Next() {
  var p AdminPolicy
  var seconds, trialSeconds int64
  if err:=rows.Scan(&p.ID,&p.PlanID,&p.Enabled,&p.Priority,&p.Region,
   &p.AssignmentTrigger,&p.ActivationTrigger,&seconds,&trialSeconds);err!=nil{return nil,err}
  p.ReservationTTL=time.Duration(seconds)*time.Second
  p.TrialDuration=time.Duration(trialSeconds)*time.Second
  result=append(result,p)
 }
 return result,rows.Err()
}

// SetEnabled allows disabling future assignments without deleting existing
// claims. It does not revoke already assigned or activated subscriptions.
func (s PolicyStore) SetEnabled(ctx context.Context,id uuid.UUID,enabled bool) error {
 if s.DB==nil {return errors.New("starter policy store has no database")}
 if id==uuid.Nil {return ErrInvalidPolicy}
 tag,err:=s.DB.Exec(ctx,`UPDATE starter_plan_policies policy SET enabled=$2,
 updated_at=now() WHERE policy.id=$1 AND (
 $2=false OR EXISTS(SELECT 1 FROM plans p WHERE p.id=policy.plan_id
 AND p.price_minor=0 AND p.active=true AND p.archived_at IS NULL
 AND (p.publish_at IS NULL OR p.publish_at<=now())))`,id,enabled)
 if err!=nil{return err}
 if tag.RowsAffected()==0 {return ErrNotEligible}
 return nil
}
