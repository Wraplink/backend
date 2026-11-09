package starterpolicy

import (
 "context"
 "errors"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5/pgxpool"
)

// AdminPolicy is the persisted configuration used by the admin policy editor.
// HTTP authentication and authorization must be enforced by the existing
// backend admin middleware before calling these methods.
type AdminPolicy struct {
 ID uuid.UUID
 PlanID uuid.UUID
 Enabled bool
 Priority int
 Region string
 AssignmentTrigger Trigger
 ActivationTrigger string
 ReservationTTL time.Duration
}

type PolicyStore struct { DB *pgxpool.Pool }

func validateAdminPolicy(p AdminPolicy) error {
 if p.PlanID==uuid.Nil || p.ReservationTTL<=0 || p.ReservationTTL%time.Second!=0 {
  return ErrInvalidPolicy
 }
 if p.AssignmentTrigger!=AfterVerification && p.AssignmentTrigger!=FirstLogin {
  return ErrInvalidPolicy
 }
 switch p.ActivationTrigger {
 case "manual", "first_dns_use", "immediate":
 default: return ErrInvalidPolicy
 }
 return nil
}

// Create inserts a disabled-by-default policy unless Enabled is explicitly set.
// A plan's zero-price and starter eligibility MUST be verified by the caller
// against the authoritative plan catalog before invoking this method.
func (s PolicyStore) Create(ctx context.Context,p AdminPolicy) (AdminPolicy,error) {
 if s.DB==nil {return AdminPolicy{},errors.New("starter policy store has no database")}
 if err:=validateAdminPolicy(p);err!=nil{return AdminPolicy{},err}
 if p.ID==uuid.Nil {p.ID=uuid.New()}
 err:=s.DB.QueryRow(ctx,`INSERT INTO starter_plan_policies
 (id,plan_id,enabled,priority,region,assignment_trigger,activation_trigger,reservation_ttl_seconds)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
 RETURNING id`,p.ID,p.PlanID,p.Enabled,p.Priority,p.Region,p.AssignmentTrigger,p.ActivationTrigger,int64(p.ReservationTTL/time.Second)).Scan(&p.ID)
 return p,err
}

// List returns policy configuration in deterministic selection order.
func (s PolicyStore) List(ctx context.Context) ([]AdminPolicy,error) {
 if s.DB==nil {return nil,errors.New("starter policy store has no database")}
 rows,err:=s.DB.Query(ctx,`SELECT id,plan_id,enabled,priority,region,
 assignment_trigger,activation_trigger,reservation_ttl_seconds
 FROM starter_plan_policies ORDER BY priority DESC,id ASC`)
 if err!=nil{return nil,err}
 defer rows.Close()
 result:=make([]AdminPolicy,0)
 for rows.Next() {
  var p AdminPolicy
  var seconds int64
  if err:=rows.Scan(&p.ID,&p.PlanID,&p.Enabled,&p.Priority,&p.Region,
   &p.AssignmentTrigger,&p.ActivationTrigger,&seconds);err!=nil{return nil,err}
  p.ReservationTTL=time.Duration(seconds)*time.Second
  result=append(result,p)
 }
 return result,rows.Err()
}

// SetEnabled allows disabling future assignments without deleting existing
// claims. It does not revoke already assigned or activated subscriptions.
func (s PolicyStore) SetEnabled(ctx context.Context,id uuid.UUID,enabled bool) error {
 if s.DB==nil {return errors.New("starter policy store has no database")}
 if id==uuid.Nil {return ErrInvalidPolicy}
 tag,err:=s.DB.Exec(ctx,`UPDATE starter_plan_policies SET enabled=$2,
 updated_at=now() WHERE id=$1`,id,enabled)
 if err!=nil{return err}
 if tag.RowsAffected()==0 {return ErrNotEligible}
 return nil
}
