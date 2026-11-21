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

 var verified bool
 err = tx.QueryRow(ctx, `
  SELECT verified_at IS NOT NULL
  FROM user_public_ips
  WHERE id=$1 AND user_id=$2 AND removed_at IS NULL FOR UPDATE
 `, addressID, userID).Scan(&verified)
 if errors.Is(err, pgx.ErrNoRows) { return ErrNotOwned }
 if err != nil { return fmt.Errorf("load selected IP: %w", err) }
 if !verified { return ErrUnverified }

 _, err = tx.Exec(ctx, `
  INSERT INTO subscription_public_ip_selections (subscription_id, user_id, public_ip_id)
  VALUES ($1,$2,$3)
  ON CONFLICT (subscription_id)
  DO UPDATE SET public_ip_id=EXCLUDED.public_ip_id, updated_at=now()
 `, subscriptionID, userID, addressID)
 if err != nil { return fmt.Errorf("save selected IP: %w", err) }
 return tx.Commit(ctx)
}
