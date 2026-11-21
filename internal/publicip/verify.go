package publicip

import (
 "context"
 "errors"
 "fmt"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)

var ErrAddressMismatch = errors.New("request public IP does not match saved address")

// VerifyObserved marks a saved IP verified only when the authenticated
// request originates from that same public IPv4 address. The observed address
// MUST be obtained from a trusted reverse-proxy boundary or direct peer;
// never pass unvalidated X-Forwarded-For or X-Real-IP header values.
func (s Store) VerifyObserved(ctx context.Context, userID, addressID uuid.UUID, observed string) error {
 ip, err := Parse(observed)
 if err != nil { return err }
 tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
 if err != nil { return fmt.Errorf("begin IP verification: %w", err) }
 defer tx.Rollback(ctx)

 var saved string
 err = tx.QueryRow(ctx, `
  SELECT host(ip_address)
  FROM user_public_ips
  WHERE id=$1 AND user_id=$2 AND removed_at IS NULL
  FOR UPDATE
 `, addressID, userID).Scan(&saved)
 if errors.Is(err, pgx.ErrNoRows) { return ErrNotOwned }
 if err != nil { return fmt.Errorf("load saved IP: %w", err) }
 if saved != ip.String() { return ErrAddressMismatch }

 _, err = tx.Exec(ctx, `
  UPDATE user_public_ips SET verified_at=COALESCE(verified_at, now()), updated_at=now()
  WHERE id=$1 AND user_id=$2 AND removed_at IS NULL
 `, addressID, userID)
 if err != nil { return fmt.Errorf("verify saved IP: %w", err) }
 _, err = tx.Exec(ctx, `
  INSERT INTO user_public_ip_events
   (id,user_id,public_ip_id,ip_address,event_type)
  VALUES ($1,$2,$3,$4::inet,'verified')
 `, uuid.New(), userID, addressID, saved)
 if err != nil { return fmt.Errorf("record verification: %w", err) }
 if err := tx.Commit(ctx); err != nil { return fmt.Errorf("commit IP verification: %w", err) }
 return nil
}
