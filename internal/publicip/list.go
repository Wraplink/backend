package publicip

import (
 "context"
 "fmt"
 "time"

 "github.com/google/uuid"
)

// Address is a saved, non-removed IPv4 address.
type Address struct {
 ID uuid.UUID `json:"id"`
 IP string `json:"ipAddress"`
 Label string `json:"label"`
 VerifiedAt *time.Time `json:"verifiedAt,omitempty"`
 CreatedAt time.Time `json:"createdAt"`
 UpdatedAt time.Time `json:"updatedAt"`
}

// List returns only non-removed addresses belonging to the authenticated user.
func (s Store) List(ctx context.Context, userID uuid.UUID) ([]Address, error) {
 rows, err := s.Pool.Query(ctx, `
  SELECT id, host(ip_address), label, verified_at, created_at, updated_at
  FROM user_public_ips
  WHERE user_id = $1 AND removed_at IS NULL
  ORDER BY created_at DESC, id DESC
 `, userID)
 if err != nil { return nil, fmt.Errorf("list saved IPs: %w", err) }
 defer rows.Close()

 result := make([]Address, 0)
 for rows.Next() {
  var a Address
  if err := rows.Scan(&a.ID, &a.IP, &a.Label, &a.VerifiedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
   return nil, fmt.Errorf("scan saved IP: %w", err)
  }
  result = append(result, a)
 }
 if err := rows.Err(); err != nil { return nil, fmt.Errorf("iterate saved IPs: %w", err) }
 return result, nil
}
