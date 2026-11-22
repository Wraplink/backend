package publicip

import (
 "context"
 "errors"
 "fmt"

 "github.com/google/uuid"
)

var ErrReconciliationEvidenceRequired = errors.New("network reconciliation evidence is required")

// ReconciliationEvidence is issued by a trusted control-plane verifier, never
// by a user request. The verifier must confirm all required DNS and edge nodes
// have fenced the old attempt before reporting a safe retry.
type ReconciliationEvidence struct {
 SubscriptionID uuid.UUID
 PublicIPID uuid.UUID
 Revision int64
 OldAttemptFenced bool
 RequiredNodesChecked bool
}

// NetworkReconciliationVerifier must independently inspect the required DNS and
// edge nodes and fence any previous attempt. A caller-provided evidence struct
// is not sufficient to authorize release.
type NetworkReconciliationVerifier interface {
 VerifyReconciliation(ctx context.Context, evidence ReconciliationEvidence) error
}

var ErrNoReconciliationVerifier = errors.New("network reconciliation verifier is not configured")

// RequeueReconciledSync only releases a quarantined revision after a trusted
// verifier has checked all required nodes and fenced stale operations.
// The verifier is mandatory; never expose this method via user-facing HTTP.
func (s Store) RequeueReconciledSync(ctx context.Context, verifier NetworkReconciliationVerifier, evidence ReconciliationEvidence) (bool,error) {
 if evidence.SubscriptionID==uuid.Nil || evidence.PublicIPID==uuid.Nil ||
    evidence.Revision<1 || !evidence.OldAttemptFenced || !evidence.RequiredNodesChecked {
  return false,ErrReconciliationEvidenceRequired
 }
 if verifier==nil{return false,ErrNoReconciliationVerifier}
 if err:=ctx.Err();err!=nil{return false,err}
 if err:=verifier.VerifyReconciliation(ctx,evidence);err!=nil{return false,fmt.Errorf("verify network reconciliation: %w",err)}
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
 `,evidence.SubscriptionID,evidence.PublicIPID,evidence.Revision)
 if err!=nil{return false,fmt.Errorf("release reconciled IP sync: %w",err)}
 return tag.RowsAffected()==1,nil
}
