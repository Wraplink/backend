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
  // The request context may already be canceled or timed out. Persist the
  // uncertain network outcome using a bounded independent context so it
  // cannot remain indefinitely in applying solely due to cancellation.
  recordCtx,cancel:=context.WithTimeout(context.WithoutCancel(ctx),5*time.Second)
  defer cancel()
  recorded,recordErr:=s.FailSync(recordCtx,job,"sync outcome uncertain; verify downstream state before retry")
  if recordErr!=nil{return true,fmt.Errorf("apply IP: %v; record failure: %w",err,recordErr)}
  if !recorded{return true,fmt.Errorf("apply IP: %w; quarantine not recorded because revision or attempt changed",err)}
  return true,fmt.Errorf("apply IP: %w",err)
 }
 // A successful network operation may outlive the worker request context.
 // Use a bounded independent context for the database acknowledgement too.
 // Never turn a database acknowledgement failure into a network retry.
 ackCtx,ackCancel:=context.WithTimeout(context.WithoutCancel(ctx),5*time.Second)
 defer ackCancel()
 acknowledged,err:=s.AckSync(ackCtx,job)
 if err!=nil{
  // AckSync might have committed before returning an ambiguous error.
  // Quarantine only the still-applying matching attempt; never overwrite an
  // already-applied row or a superseding revision.
  quarantineCtx,quarantineCancel:=context.WithTimeout(context.WithoutCancel(ctx),5*time.Second)
  defer quarantineCancel()
  _,quarantineErr:=s.FailSync(quarantineCtx,job,"network applied but database acknowledgement uncertain; reconcile before retry")
  if quarantineErr!=nil{return true,fmt.Errorf("acknowledge IP: %v; quarantine: %w",err,quarantineErr)}
  return true,fmt.Errorf("acknowledge IP: %w",err)
 }
 if !acknowledged{
  // A newer selection superseded the revision while the network request
  // was in flight. The new revision must be reconciled before reporting
  // the subscription as applied.
  return true,fmt.Errorf("%w: revision or ownership changed during application",ErrSyncAcknowledgementRejected)
 }
 return true,nil
}
