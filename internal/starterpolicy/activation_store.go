package starterpolicy

import (
 "context"
 "errors"
 "fmt"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)

// ActivationStore promotes a reserved starter claim and creates the
// corresponding active subscription in the same database transaction.
type ActivationStore struct { DB *pgxpool.Pool }

type ActivatedTrial struct {
 ClaimID uuid.UUID `json:"claimId"`
 SubscriptionID uuid.UUID `json:"subscriptionId"`
 StartsAt time.Time `json:"startsAt"`
 ExpiresAt time.Time `json:"expiresAt"`
}

var ErrActiveSubscription = errors.New("another subscription is already active")

func (s ActivationStore) Activate(ctx context.Context, userID, claimID uuid.UUID) (ActivatedTrial,error) {
 if s.DB==nil || userID==uuid.Nil || claimID==uuid.Nil {return ActivatedTrial{},ErrNotEligible}
 tx,err:=s.DB.BeginTx(ctx,pgx.TxOptions{})
 if err!=nil{return ActivatedTrial{},err}
 defer tx.Rollback(ctx)
 // The user row is a common serialization point for concurrent trial
 // activations and avoids races between different claims of this user.
 var exists uuid.UUID
 if err=tx.QueryRow(ctx,`SELECT id FROM users WHERE id=$1 FOR UPDATE`,userID).Scan(&exists);err!=nil{return ActivatedTrial{},err}

 var state string
 var deadline time.Time
 var planID uuid.UUID
 var policyID *uuid.UUID
 var oldSubscription *uuid.UUID
 err=tx.QueryRow(ctx,`SELECT state,reservation_expires_at,plan_id,starter_policy_id,subscription_id
 FROM free_plan_claims WHERE id=$1 AND user_id=$2 AND source='starter' FOR UPDATE`,claimID,userID).
 Scan(&state,&deadline,&planID,&policyID,&oldSubscription)
 if errors.Is(err,pgx.ErrNoRows){return ActivatedTrial{},ErrNotEligible}
 if err!=nil{return ActivatedTrial{},err}
 // Idempotent retries return only a still-active, unexpired subscription.
 // Never create another trial subscription for a previously activated claim.
 if oldSubscription!=nil && state=="active" {
  var result ActivatedTrial
  var subscriptionStatus string
  err=tx.QueryRow(ctx,`SELECT id,starts_at,expires_at,status FROM user_subscriptions
  WHERE id=$1 AND user_id=$2`,*oldSubscription,userID).
  Scan(&result.SubscriptionID,&result.StartsAt,&result.ExpiresAt,&subscriptionStatus)
  if errors.Is(err,pgx.ErrNoRows){return ActivatedTrial{},ErrAlreadyActivated}
  if err!=nil{return ActivatedTrial{},err}
  if subscriptionStatus!="active" {return ActivatedTrial{},ErrAlreadyActivated}
  result.ClaimID=claimID
  if !result.ExpiresAt.After(time.Now().UTC()) {return ActivatedTrial{},ErrAlreadyActivated}
  if err=tx.Commit(ctx);err!=nil{return ActivatedTrial{},err}
  return result,nil
 }
 if state!="reserved" || policyID==nil {return ActivatedTrial{},ErrAlreadyActivated}
 var durationSeconds int64
 err=tx.QueryRow(ctx,`SELECT trial_duration_seconds FROM starter_plan_policies WHERE id=$1`,*policyID).Scan(&durationSeconds)
 if err!=nil{return ActivatedTrial{},err}
 now:=time.Now().UTC()
 window,err:=Window(now,deadline,time.Duration(durationSeconds)*time.Second,state)
 if err!=nil{return ActivatedTrial{},err}
 var active bool
 err=tx.QueryRow(ctx,`SELECT EXISTS(SELECT 1 FROM user_subscriptions
 WHERE user_id=$1 AND status='active' AND starts_at <= now() AND expires_at > now())`,userID).Scan(&active)
 if err!=nil{return ActivatedTrial{},err}
 if active{return ActivatedTrial{},ErrActiveSubscription}

 result:=ActivatedTrial{ClaimID:claimID,StartsAt:window.StartsAt,ExpiresAt:window.ExpiresAt}
 err=tx.QueryRow(ctx,`INSERT INTO user_subscriptions
 (user_id,plan_id,plan_code,plan_name,price_minor,currency,duration_days,data_limit_bytes,
 status,starts_at,expires_at,activated_at)
 SELECT $1,p.id,p.code,p.name,p.price_minor,p.currency,p.duration_days,p.data_limit_bytes,
 'active',$3,$4,$3
 FROM plans p WHERE p.id=$2 AND p.price_minor=0 AND p.active
 AND p.archived_at IS NULL AND (p.publish_at IS NULL OR p.publish_at <= now())
 RETURNING id`,userID,planID,window.StartsAt,window.ExpiresAt).Scan(&result.SubscriptionID)
 if errors.Is(err,pgx.ErrNoRows){return ActivatedTrial{},ErrNotEligible}
 if err!=nil{return ActivatedTrial{},fmt.Errorf("create trial subscription: %w",err)}
 tag,err:=tx.Exec(ctx,`UPDATE free_plan_claims
 SET state='active',activated_at=$3,subscription_id=$4
 WHERE id=$1 AND user_id=$2 AND state='reserved'`,claimID,userID,window.StartsAt,result.SubscriptionID)
 if err!=nil{return ActivatedTrial{},err}
 if tag.RowsAffected()!=1{return ActivatedTrial{},ErrAlreadyActivated}
 if err=tx.Commit(ctx);err!=nil{return ActivatedTrial{},err}
 return result,nil
}


// MyStarterClaims returns durable claims, including expired reservations,
// so users can see why an already-claimed offer is not available again.
type MyStarterClaim struct {
 ID uuid.UUID `json:"id"`
 PlanID uuid.UUID `json:"planId"`
 State string `json:"state"`
 ReservationExpiresAt *time.Time `json:"reservationExpiresAt,omitempty"`
 ActivatedAt *time.Time `json:"activatedAt,omitempty"`
 SubscriptionID *uuid.UUID `json:"subscriptionId,omitempty"`
 StartsAt *time.Time `json:"startsAt,omitempty"`
 ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}
func (s ActivationStore) MyStarterClaims(ctx context.Context,userID uuid.UUID)([]MyStarterClaim,error){
 if s.DB==nil || userID==uuid.Nil{return nil,ErrNotEligible}
 rows,err:=s.DB.Query(ctx,`SELECT c.id,c.plan_id,c.state,c.reservation_expires_at,c.activated_at,c.subscription_id,s.starts_at,s.expires_at,s.status
 FROM free_plan_claims c LEFT JOIN user_subscriptions s ON s.id=c.subscription_id AND s.user_id=c.user_id
 WHERE c.user_id=$1 AND c.source='starter' ORDER BY c.assigned_at DESC`,userID)
 if err!=nil{return nil,err}
 defer rows.Close()
 result:=make([]MyStarterClaim,0)
 for rows.Next(){
  var item MyStarterClaim
  var subscriptionStatus *string
  if err:=rows.Scan(&item.ID,&item.PlanID,&item.State,&item.ReservationExpiresAt,&item.ActivatedAt,&item.SubscriptionID,&item.StartsAt,&item.ExpiresAt,&subscriptionStatus);err!=nil{return nil,err}
  if item.State=="active" && (subscriptionStatus==nil || *subscriptionStatus!="active" || item.ExpiresAt==nil) {item.State="cancelled"}
  if item.State=="active" && item.StartsAt!=nil && time.Now().Before(*item.StartsAt) {item.State="failed"}
  if item.State=="reserved" && item.ReservationExpiresAt!=nil && !time.Now().Before(*item.ReservationExpiresAt){item.State="expired"}
  if item.State=="active" && item.ExpiresAt!=nil && !time.Now().Before(*item.ExpiresAt){item.State="expired"}
  result=append(result,item)
 }
 return result,rows.Err()
}
