package publicip

import (
 "context"
 "errors"
 "fmt"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)

// SyncStatus describes desired and acknowledged revisions. A pending
// selection is not proof that DNS Control or any Edge Agent applied it.
type SyncStatus struct {
 SubscriptionID uuid.UUID `json:"subscriptionId"`
 DesiredPublicIPID *uuid.UUID `json:"desiredPublicIpId"`
 AppliedPublicIPID *uuid.UUID `json:"appliedPublicIpId"`
 Revision int64 `json:"revision"`
 AppliedRevision int64 `json:"appliedRevision"`
 Status string `json:"status"`
 RequiresReconciliation bool `json:"requiresReconciliation"`
 UpdatedAt *time.Time `json:"updatedAt"`
}

// GetSyncStatus reads a subscription's reconciliation status, scoped to owner.
// Do not expose internal last_error messages to clients.
func (s Store) GetSyncStatus(ctx context.Context, userID, subscriptionID uuid.UUID) (SyncStatus, error) {
 var result SyncStatus
 err := s.Pool.QueryRow(ctx, `
  SELECT subscription.id, sync.desired_public_ip_id,
         sync.applied_public_ip_id, COALESCE(sync.revision,0), COALESCE(sync.applied_revision,0), COALESCE(sync.status,'unconfigured'), sync.updated_at,
         (sync.status = 'failed' AND COALESCE(sync.last_error,'') LIKE 'sync attempt expired;%')
  FROM user_subscriptions AS subscription
  LEFT JOIN subscription_public_ip_sync AS sync
    ON sync.subscription_id = subscription.id
  WHERE subscription.id=$1 AND subscription.user_id=$2
 `, subscriptionID, userID).Scan(
  &result.SubscriptionID, &result.DesiredPublicIPID, &result.AppliedPublicIPID,
  &result.Revision, &result.AppliedRevision, &result.Status, &result.UpdatedAt,
  &result.RequiresReconciliation,
 )
 if errors.Is(err, pgx.ErrNoRows) { return SyncStatus{}, ErrSubscriptionNotOwned }
 if err != nil { return SyncStatus{}, fmt.Errorf("read IP sync status: %w", err) }
 return result, nil
}
