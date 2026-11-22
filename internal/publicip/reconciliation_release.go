package publicip

import (
 "context"
 "errors"
 "fmt"

 "github.com/google/uuid"
)

var ErrReconciliationEvidenceRequired = errors.New("network reconciliation evidence is required")

// NetworkReconciliationVerifier must independently inspect the required DNS and
// edge nodes and durably fence any previous attempt. The caller supplies only
// the target identity; safety evidence must be obtained independently.
type NetworkReconciliationVerifier interface {
 VerifyReconciliation(ctx context.Context, subscriptionID, publicIPID uuid.UUID, revision int64) error
}

var ErrNoReconciliationVerifier = errors.New("network reconciliation verifier is not configured")

// RequeueReconciledSync only releases a quarantined revision after a trusted
// verifier has checked all required nodes and fenced stale operations.
// The verifier is mandatory; never expose this method via user-facing HTTP.
func (s Store) RequeueReconciledSync(ctx context.Context, verifier NetworkReconciliationVerifier, subscriptionID, publicIPID uuid.UUID, revision int64) (bool,error) {
 if subscriptionID==uuid.Nil || publicIPID==uuid.Nil || revision<1 {
  return false,ErrReconciliationEvidenceRequired
 }
 if verifier==nil{return false,ErrNoReconciliationVerifier}
 if err:=ctx.Err();err!=nil{return false,err}
 if err:=verifier.VerifyReconciliation(ctx,subscriptionID,publicIPID,revision);err!=nil{return false,fmt.Errorf("verify network reconciliation: %w",err)}
 if err:=ctx.Err();err!=nil{return false,err}
 tag,err:=s.Pool.Exec(ctx,`
  UPDATE subscription_public_ip_sync AS sync
  SET status='pending',attempt_id=NULL,last_error=NULL,updated_at=now()
  FROM subscription_public_ip_selections AS selection
  JOIN user_public_ips AS ip ON ip.id=selection.public_ip_id
  JOIN user_subscriptions AS subscription ON subscription.id=selection.subscription_id
  WHERE sync.subscription_id=$1 AND sync.desired_public_ip_id=$2
    AND sync.revision=$3 AND sync.status='needs_reconciliation'
    AND selection.subscription_id=sync.subscription_id
    AND selection.public_ip_id=sync.desired_public_ip_id
    AND selection.user_id=subscription.user_id
    AND ip.user_id=subscription.user_id
    AND ip.verified_at IS NOT NULL AND ip.removed_at IS NULL
 `,subscriptionID,publicIPID,revision)
 if err!=nil{return false,fmt.Errorf("release reconciled IP sync: %w",err)}
 return tag.RowsAffected()==1,nil
}
