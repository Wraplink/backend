package publicip

import (
 "context"
 "errors"
 "fmt"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgxpool"
)

// Store persists saved IP addresses. All queries are scoped to the authenticated user.
type Store struct { Pool *pgxpool.Pool }

// Remove soft-deletes an inactive saved address and writes an immutable audit event.
// Activation must lock the same user_public_ips row before updating selections.
func (s Store) Remove(ctx context.Context, userID, addressID uuid.UUID) error {
 tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
 if err != nil { return fmt.Errorf("begin IP removal: %w", err) }
 defer tx.Rollback(ctx)

 var address string
 var removed bool
 err = tx.QueryRow(ctx, `
  SELECT host(ip_address), removed_at IS NOT NULL
  FROM user_public_ips
  WHERE id = $1 AND user_id = $2
  FOR UPDATE
 `, addressID, userID).Scan(&address, &removed)
 if errors.Is(err, pgx.ErrNoRows) { return ErrNotOwned }
 if err != nil { return fmt.Errorf("load IP: %w", err) }

 var active int
 err = tx.QueryRow(ctx, `
  SELECT (SELECT count(*) FROM subscription_public_ip_selections WHERE public_ip_id=$1)
       + (SELECT count(*) FROM subscription_public_ip_sync WHERE applied_public_ip_id=$1)
       + (SELECT count(*) FROM subscription_public_ip_sync WHERE desired_public_ip_id=$1 AND status IN ('pending','applying','failed'))
       + (SELECT count(*) FROM network_entitlements WHERE user_id=$2 AND client_ip=$3::inet AND status='active')
 `, addressID,userID,address).Scan(&active)
 if err != nil { return fmt.Errorf("check active selections: %w", err) }
 if err = CanRemove(true, removed, active); err != nil { return err }

 tag, err := tx.Exec(ctx, `
  UPDATE user_public_ips
  SET removed_at = now(), updated_at = now()
  WHERE id = $1 AND user_id = $2 AND removed_at IS NULL
 `, addressID, userID)
 if err != nil { return fmt.Errorf("soft-delete IP: %w", err) }
 if tag.RowsAffected() != 1 { return ErrNotOwned }

 _, err = tx.Exec(ctx, `
  INSERT INTO user_public_ip_events
   (id, user_id, public_ip_id, ip_address, event_type)
  VALUES ($1, $2, $3, $4::inet, 'removed')
 `, uuid.New(), userID, addressID, address)
 if err != nil { return fmt.Errorf("record IP removal: %w", err) }
 if err = tx.Commit(ctx); err != nil { return fmt.Errorf("commit IP removal: %w", err) }
 return nil
}
