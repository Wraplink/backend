package starterpolicy

import (
 "context"
 "errors"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotEligible = errors.New("starter offer is not eligible")

// ReservationStore atomically creates at most one automatic starter reservation
// for a verified new user. It does not create a subscription or grant network
// entitlements. Call after the verification transaction commits.
type ReservationStore struct { DB *pgxpool.Pool }

type Reservation struct {
 ClaimID uuid.UUID
 PlanID uuid.UUID
 ExpiresAt time.Time
 Created bool
}

func (s ReservationStore) ReserveVerified(ctx context.Context, userID uuid.UUID, region string, newUser bool) (Reservation,error) {
 if s.DB==nil || userID==uuid.Nil || !newUser { return Reservation{},ErrNotEligible }
 tx,err:=s.DB.BeginTx(ctx,pgx.TxOptions{})
 if err!=nil { return Reservation{},err }
 defer tx.Rollback(ctx)

 // Serialize competing callbacks for the same account, including callbacks
 // racing to select different policies. Locking users also ensures existence.
 // This helper is only safe for onboarding callers that independently prove
 // newUser from a trusted first-verification transition; do not expose it
 // directly to client-controlled requests.
 var verified bool
 err=tx.QueryRow(ctx,`SELECT email_verified FROM users WHERE id=$1 FOR UPDATE`,userID).Scan(&verified)
 if err!=nil { return Reservation{},err }
 if !verified { return Reservation{},ErrNotEligible }

 var existing Reservation
 var existingDeadline *time.Time
 err=tx.QueryRow(ctx,`SELECT id,plan_id,reservation_expires_at
 FROM free_plan_claims WHERE user_id=$1 AND source='starter'`,userID).
 Scan(&existing.ClaimID,&existing.PlanID,&existingDeadline)
 if err==nil {
  if existingDeadline==nil {return Reservation{},ErrNotEligible}
  existing.ExpiresAt=*existingDeadline
  if err=tx.Commit(ctx);err!=nil{return Reservation{},err}
  return existing,nil
 }
 if !errors.Is(err,pgx.ErrNoRows) { return Reservation{},err }

 var policyID,planID uuid.UUID
 var ttl int64
 err=tx.QueryRow(ctx,`SELECT policy.id,policy.plan_id,policy.reservation_ttl_seconds
 FROM starter_plan_policies policy JOIN plans p ON p.id=policy.plan_id
 WHERE p.price_minor=0 AND p.active=true AND p.archived_at IS NULL
 AND (p.publish_at IS NULL OR p.publish_at<=now())
 AND policy.enabled AND policy.assignment_trigger='email_verified'
 AND (policy.region='' OR lower(policy.region)=lower($1))
 AND NOT EXISTS (
  SELECT 1 FROM free_plan_claims claimed
  WHERE claimed.user_id=$2 AND (claimed.plan_id=policy.plan_id OR lower(claimed.plan_code)=lower(p.code))
 )
 ORDER BY policy.priority DESC,policy.id ASC LIMIT 1`,region,userID).Scan(&policyID,&planID,&ttl)
 if errors.Is(err,pgx.ErrNoRows) {
  if err=tx.Commit(ctx);err!=nil{return Reservation{},err}
  return Reservation{},ErrNotEligible
 }
 if err!=nil{return Reservation{},err}

 claimID:=uuid.New()
 err=tx.QueryRow(ctx,`INSERT INTO free_plan_claims
 (id,user_id,plan_id,plan_code,starter_policy_id,source,state,reservation_expires_at)
 SELECT $1,$2,$3,p.code,$4,'starter','reserved',now()+($5 * interval '1 second') FROM plans p WHERE p.id=$3
 ON CONFLICT DO NOTHING
 RETURNING id,plan_id,reservation_expires_at`,claimID,userID,planID,policyID,ttl).
 Scan(&existing.ClaimID,&existing.PlanID,&existingDeadline)
 if errors.Is(err,pgx.ErrNoRows) {
  err=tx.QueryRow(ctx,`SELECT id,plan_id,reservation_expires_at
 FROM free_plan_claims WHERE user_id=$1 AND source='starter'`,userID).
 Scan(&existing.ClaimID,&existing.PlanID,&existingDeadline)
  if errors.Is(err,pgx.ErrNoRows) {
   // The plan may have been claimed through the market concurrently.
   // Do not issue a second starter entitlement or overwrite its claim.
   return Reservation{},ErrNotEligible
  }
  if err!=nil{return Reservation{},err}
 } else if err!=nil{return Reservation{},err
 } else {existing.Created=true}
 if existingDeadline==nil {return Reservation{},ErrNotEligible}
 existing.ExpiresAt=*existingDeadline
 if err=tx.Commit(ctx);err!=nil{return Reservation{},err}
 return existing,nil
}
