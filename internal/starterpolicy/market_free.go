package starterpolicy

import (
 "context"
 "errors"
 "fmt"
 "strings"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)

var ErrFreePlanClaimed=errors.New("free plan already claimed")
var ErrFreePlanUnavailable=errors.New("free plan not available")

type MarketFreeStore struct{DB *pgxpool.Pool}
type MarketFreeResult struct{
 ClaimID uuid.UUID `json:"claimId"`
 SubscriptionID uuid.UUID `json:"subscriptionId"`
 StartsAt time.Time `json:"startsAt"`
 ExpiresAt time.Time `json:"expiresAt"`
}

// Claim atomically grants one zero-cost market plan, never charging a wallet.
// Claims remain after cancellation or expiry and cannot be reissued.
func (s MarketFreeStore) Claim(ctx context.Context,userID uuid.UUID,planCode string)(MarketFreeResult,error){
 planCode=strings.ToLower(strings.TrimSpace(planCode))
 if s.DB==nil || userID==uuid.Nil || planCode==""{return MarketFreeResult{},ErrFreePlanUnavailable}
 tx,err:=s.DB.BeginTx(ctx,pgx.TxOptions{})
 if err!=nil{return MarketFreeResult{},err}
 defer tx.Rollback(ctx)
 var verified bool
 if err=tx.QueryRow(ctx,`SELECT email_verified FROM users WHERE id=$1 FOR UPDATE`,userID).Scan(&verified);err!=nil{return MarketFreeResult{},err}
 if !verified{return MarketFreeResult{},ErrNotEligible}
 var planID uuid.UUID
 var days int
 err=tx.QueryRow(ctx,`SELECT id,duration_days FROM plans
 WHERE lower(code)=$1 AND active AND price_minor=0 AND archived_at IS NULL
 AND (publish_at IS NULL OR publish_at<=now()) FOR SHARE`,planCode).Scan(&planID,&days)
 if errors.Is(err,pgx.ErrNoRows){return MarketFreeResult{},ErrFreePlanUnavailable}
 if err!=nil{return MarketFreeResult{},err}
 if days<=0{return MarketFreeResult{},ErrFreePlanUnavailable}
 var existing bool
 if err=tx.QueryRow(ctx,`SELECT EXISTS(SELECT 1 FROM free_plan_claims WHERE user_id=$1 AND plan_id=$2)`,userID,planID).Scan(&existing);err!=nil{return MarketFreeResult{},err}
 if existing{return MarketFreeResult{},ErrFreePlanClaimed}
 if err=tx.QueryRow(ctx,`SELECT EXISTS(SELECT 1 FROM free_plan_claims WHERE user_id=$1 AND lower(plan_code)=$2)`,userID,planCode).Scan(&existing);err!=nil{return MarketFreeResult{},err}
 if existing{return MarketFreeResult{},ErrFreePlanClaimed}
 if err=tx.QueryRow(ctx,`SELECT EXISTS(SELECT 1 FROM user_subscriptions WHERE user_id=$1 AND status='active' AND starts_at<=now() AND expires_at>now())`,userID).Scan(&existing);err!=nil{return MarketFreeResult{},err}
 if existing{return MarketFreeResult{},ErrActiveSubscription}
 result:=MarketFreeResult{ClaimID:uuid.New(),StartsAt:time.Now().UTC()}
 result.ExpiresAt=result.StartsAt.AddDate(0,0,days)
 // First reserve the durable claim. A concurrent request can never grant
 // another subscription even if it races across multiple API workers.
 tag,err:=tx.Exec(ctx,`INSERT INTO free_plan_claims(id,user_id,plan_id,plan_code,source,state,activated_at)
 VALUES($1,$2,$3,$5,'market','activating',$4) ON CONFLICT DO NOTHING`,result.ClaimID,userID,planID,result.StartsAt,planCode)
 if err!=nil{return MarketFreeResult{},err}
 if tag.RowsAffected()!=1{return MarketFreeResult{},ErrFreePlanClaimed}
 err=tx.QueryRow(ctx,`INSERT INTO user_subscriptions
 (user_id,plan_id,plan_code,plan_name,plan_description,price_minor,currency,duration_days,data_limit_bytes,
 status,starts_at,expires_at,activated_at)
 SELECT $1,p.id,p.code,p.name,p.description,0,p.currency,p.duration_days,p.data_limit_bytes,
 'active',$3,$4,$3 FROM plans p
 WHERE p.id=$2 AND p.price_minor=0 AND p.active
 AND p.archived_at IS NULL AND (p.publish_at IS NULL OR p.publish_at<=now())
 RETURNING id`,userID,planID,result.StartsAt,result.ExpiresAt).Scan(&result.SubscriptionID)
 if errors.Is(err,pgx.ErrNoRows){return MarketFreeResult{},ErrFreePlanUnavailable}
 if err!=nil{return MarketFreeResult{},fmt.Errorf("create free subscription: %w",err)}
 updateTag,err:=tx.Exec(ctx,`UPDATE free_plan_claims
 SET state='active',subscription_id=$2
 WHERE id=$1 AND user_id=$3 AND state='activating' AND subscription_id IS NULL`,result.ClaimID,result.SubscriptionID,userID)
 if err!=nil{return MarketFreeResult{},err}
 if updateTag.RowsAffected()!=1{return MarketFreeResult{},ErrFreePlanClaimed}
 if err=tx.Commit(ctx);err!=nil{return MarketFreeResult{},err}
 return result,nil
}
