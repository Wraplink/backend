package publicip

import (
 "context"
 "errors"
 "fmt"
 "strings"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
 "github.com/jackc/pgx/v5/pgconn"
)

// Add saves an address without granting any network access. Verification and
// subscription activation are separate operations.
func (s Store) Add(ctx context.Context, userID uuid.UUID, rawIP, label string) (uuid.UUID, error) {
 ip, err := Parse(rawIP)
 if err != nil { return uuid.Nil, err }
 label = strings.TrimSpace(label)
 if len([]rune(label)) > 100 { return uuid.Nil, ErrInvalidLabel }

 tx, err := s.Pool.Begin(ctx)
 if err != nil { return uuid.Nil, fmt.Errorf("begin add IP: %w", err) }
 defer tx.Rollback(ctx)
 id := uuid.New()
 _, err = tx.Exec(ctx, `
  INSERT INTO user_public_ips (id, user_id, ip_address, label)
  VALUES ($1, $2, $3::inet, $4)
 `, id, userID, ip.String(), label)
 if err != nil {
  var pgErr *pgconn.PgError
  if errors.As(err, &pgErr) && pgErr.Code == "23505" { return uuid.Nil, ErrDuplicateAddress }
  return uuid.Nil, fmt.Errorf("save IP: %w", err)
 }
 _, err = tx.Exec(ctx, `
  INSERT INTO user_public_ip_events (id, user_id, public_ip_id, ip_address, event_type)
  VALUES ($1, $2, $3, $4::inet, 'added')
 `, uuid.New(), userID, id, ip.String())
 if err != nil { return uuid.Nil, fmt.Errorf("record added IP: %w", err) }
 if err := tx.Commit(ctx); err != nil { return uuid.Nil, fmt.Errorf("commit added IP: %w", err) }
 return id, nil
}

var (
 ErrDuplicateAddress = errors.New("this IP is already saved")
 ErrInvalidLabel = errors.New("label must not exceed 100 characters")
)

// Rename changes the label of a saved address without affecting entitlements.
func (s Store) Rename(ctx context.Context, userID, addressID uuid.UUID, label string) error {
 label = strings.TrimSpace(label)
 if len([]rune(label)) > 100 { return ErrInvalidLabel }
 tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
 if err != nil { return fmt.Errorf("begin rename IP: %w", err) }
 defer tx.Rollback(ctx)
 var address string
 err = tx.QueryRow(ctx, `
  SELECT host(ip_address) FROM user_public_ips
  WHERE id=$1 AND user_id=$2 AND removed_at IS NULL FOR UPDATE
 `, addressID, userID).Scan(&address)
 if errors.Is(err, pgx.ErrNoRows) { return ErrNotOwned }
 if err != nil { return fmt.Errorf("load IP: %w", err) }
 _, err = tx.Exec(ctx, `
  UPDATE user_public_ips SET label=$3, updated_at=now()
  WHERE id=$1 AND user_id=$2
 `, addressID, userID, label)
 if err != nil { return fmt.Errorf("rename IP: %w", err) }
 _, err = tx.Exec(ctx, `
  INSERT INTO user_public_ip_events (id,user_id,public_ip_id,ip_address,event_type)
  VALUES ($1,$2,$3,$4::inet,'renamed')
 `, uuid.New(), userID, addressID, address)
 if err != nil { return fmt.Errorf("record IP rename: %w", err) }
 return tx.Commit(ctx)
}
