package publicip

import (
 "context"
 "errors"
 "fmt"
 "time"
)

// EntitlementApplier must be implemented by the existing control-plane
// integration. Apply must be idempotent for (subscription, revision), and must
// confirm the new IP is active across required nodes before returning nil.
// It must NOT revoke the previous working address before confirmation.
type EntitlementApplier interface {
 ApplyPublicIP(ctx context.Context, job SyncJob) error
}

var ErrNoEntitlementApplier = errors.New("public IP entitlement applier is not configured")
var ErrSyncAcknowledgementRejected = errors.New("public IP synchronization acknowledgement rejected")

// ReconcileOne processes a single pending request. No network operation is
// performed if an applier is absent. An uncertain apply is quarantined as
// needs_reconciliation and must not be automatically retried. The worker
// does not infer success.
func (s Store) ReconcileOne(ctx context.Context, applier EntitlementApplier) (bool,error) {
 if applier==nil{return false,ErrNoEntitlementApplier}
 job,found,err:=s.ClaimSync(ctx)
 if err!=nil{return false,err}
 if !found{return false,nil}
 if err=applier.ApplyPublicIP(ctx,job);err!=nil{
  // The request context may already be canceled or timed out. Persist the\n  // uncertain network outcome using a bounded independent context so it\n  // cannot remain indefinitely in applying solely due to cancellation.\n  recordCtx,cancel:=context.WithTimeout(context.WithoutCancel(ctx),5*time.Second)\n  defer cancel()\n  recorded,recordErr:=s.FailSync(recordCtx,job,"sync outcome uncertain; verify downstream state before retry")
  if recordErr!=nil{return true,fmt.Errorf("apply IP: %v; record failure: %w",err,recordErr)}
  if !recorded{return true,fmt.Errorf("apply IP: %w; quarantine not recorded because revision or attempt changed",err)}\n  return true,fmt.Errorf("apply IP: %w",err)
 }
 acknowledged,err:=s.AckSync(ctx,job)
 if err!=nil{return true,err}
 if !acknowledged{
  // A newer selection superseded the revision while the network request
  // was in flight. The new revision must be reconciled before reporting
  // the subscription as applied.
  return true,fmt.Errorf("%w: revision or ownership changed during application",ErrSyncAcknowledgementRejected)
 }
 return true,nil
}
