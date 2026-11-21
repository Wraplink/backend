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
 DesiredPublicIPID uuid.UUID `json:"desiredPublicIpId"`
 Revision int64 `json:"revision"`
 AppliedRevision int64 `json:"appliedRevision"`
 Status string `json:"status"`
 UpdatedAt time.Time `json:"updatedAt"`
}

// GetSyncStatus reads a subscription's reconciliation status, scoped to owner.
// Do not expose internal last_error messages to clients.
func (s Store) GetSyncStatus(ctx context.Context, userID, subscriptionID uuid.UUID) (SyncStatus, error) {
 var result SyncStatus
 err := s.Pool.QueryRow(ctx, `
  SELECT sync.subscription_id, sync.desired_public_ip_id,
         sync.revision, sync.applied_revision, sync.status, sync.updated_at
  FROM subscription_public_ip_sync AS sync
  JOIN user_subscriptions AS subscription
    ON subscription.id = sync.subscription_id
  WHERE subscription.id=$1 AND subscription.user_id=$2
 `, subscriptionID, userID).Scan(
  &result.SubscriptionID, &result.DesiredPublicIPID,
  &result.Revision, &result.AppliedRevision, &result.Status, &result.UpdatedAt,
 )
 if errors.Is(err, pgx.ErrNoRows) { return SyncStatus{}, ErrSubscriptionNotOwned }
 if err != nil { return SyncStatus{}, fmt.Errorf("read IP sync status: %w", err) }
 return result, nil
}
