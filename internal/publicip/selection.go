package publicip

import (
 "context"
 "errors"
 "fmt"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)

var ErrUnverified = errors.New("verify this IP before activation")
var ErrSubscriptionNotOwned = errors.New("subscription not found")
var ErrSyncInProgress = errors.New("public IP synchronization is in progress")
var ErrSyncRequiresReconciliation = errors.New("previous public IP sync requires network reconciliation")

// SelectVerified records a verified IP preference for a subscription.
// IMPORTANT: this does not publish entitlements. Until the network sync
// integration is complete, callers MUST NOT expose this as an activation API.
// The IP row is locked to serialize against Remove.
func (s Store) SelectVerified(ctx context.Context, userID, subscriptionID, addressID uuid.UUID) error {
 tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
 if err != nil { return fmt.Errorf("begin IP selection: %w", err) }
 defer tx.Rollback(ctx)

 // Serialize changes for the subscription, and ensure it belongs to the user.
 var ownedSubscription uuid.UUID
 err = tx.QueryRow(ctx, `
  SELECT id FROM user_subscriptions WHERE id=$1 AND user_id=$2 FOR UPDATE
 `, subscriptionID, userID).Scan(&ownedSubscription)
 if errors.Is(err, pgx.ErrNoRows) { return ErrSubscriptionNotOwned }
 if err != nil { return fmt.Errorf("load subscription: %w", err) }

 var selectedAddress string
 var verified bool
 err = tx.QueryRow(ctx, `
  SELECT host(ip_address), verified_at IS NOT NULL
  FROM user_public_ips
  WHERE id=$1 AND user_id=$2 AND removed_at IS NULL FOR UPDATE
 `, addressID, userID).Scan(&selectedAddress, &verified)
 if errors.Is(err, pgx.ErrNoRows) { return ErrNotOwned }
 if err != nil { return fmt.Errorf("load selected IP: %w", err) }
 if !verified { return ErrUnverified }

 // Repeated requests for the same desired address must not supersede an
 // in-flight attempt or generate a new revision. A failed attempt remains
 // failed until an explicit retry; do not silently hide its error.
 var desiredID uuid.UUID
 var syncStatus string
 var requiresReconciliation bool
 err = tx.QueryRow(ctx,`
  SELECT desired_public_ip_id,status,
    (status='needs_reconciliation')
  FROM subscription_public_ip_sync
  WHERE subscription_id=$1 FOR UPDATE
 `,subscriptionID).Scan(&desiredID,&syncStatus,&requiresReconciliation)
 if err!=nil && !errors.Is(err,pgx.ErrNoRows) {
  return fmt.Errorf("load existing IP sync: %w",err)
 }
 if err==nil && requiresReconciliation {return ErrSyncRequiresReconciliation}
 if err==nil && desiredID==addressID {
  return nil
 }
 // Never supersede an applying revision: its external side effects may
 // still be in progress even if its database acknowledgement is fenced.
 if err==nil && syncStatus=="applying" {return ErrSyncInProgress}

 _, err = tx.Exec(ctx, `
  INSERT INTO subscription_public_ip_selections (subscription_id, user_id, public_ip_id)
  VALUES ($1,$2,$3)
  ON CONFLICT (subscription_id)
  DO UPDATE SET public_ip_id=EXCLUDED.public_ip_id, updated_at=now()
 `, subscriptionID, userID, addressID)
 if err != nil { return fmt.Errorf("save selected IP: %w", err) }
 // Enqueue desired state in the same transaction as the preference change.
 // The network reconciler must only acknowledge a revision after DNS and edge
 // nodes have applied the new entitlement. Never revoke an old IP here.
 _, err = tx.Exec(ctx, `
  INSERT INTO subscription_public_ip_sync
   (subscription_id, desired_public_ip_id, revision, applied_revision, status)
  VALUES ($1, $2, 1, 0, 'pending')
  ON CONFLICT (subscription_id) DO UPDATE
  SET desired_public_ip_id=EXCLUDED.desired_public_ip_id,
      revision=subscription_public_ip_sync.revision+1,
      status='pending', attempt_id=NULL, last_error=NULL, updated_at=now()
 `, subscriptionID, addressID)
 if err != nil { return fmt.Errorf("enqueue IP sync: %w", err) }

 _, err = tx.Exec(ctx, `
  INSERT INTO user_public_ip_events
    (id, user_id, public_ip_id, ip_address, event_type, subscription_id)
  VALUES ($1, $2, $3, $4::inet, 'selected', $5)
 `, uuid.New(), userID, addressID, selectedAddress, subscriptionID)
 if err != nil { return fmt.Errorf("record IP selection: %w", err) }
 if err := tx.Commit(ctx); err != nil { return fmt.Errorf("commit IP selection: %w", err) }
 return nil
}
