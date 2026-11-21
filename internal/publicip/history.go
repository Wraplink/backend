package publicip

import (
 "context"
 "fmt"
 "time"

 "github.com/google/uuid"
)

type Event struct {
 ID uuid.UUID `json:"id"`
 PublicIPID *uuid.UUID `json:"publicIpId,omitempty"`
 IPAddress string `json:"ipAddress"`
 Type string `json:"type"`
 SubscriptionID *uuid.UUID `json:"subscriptionId,omitempty"`
 CreatedAt time.Time `json:"createdAt"`
}

// History returns immutable events for the authenticated user, including removed IPs.
// A bounded limit avoids unbounded history queries.
func (s Store) History(ctx context.Context, userID uuid.UUID, limit int) ([]Event, error) {
 if limit <= 0 { limit = 50 }
 if limit > 200 { limit = 200 }
 rows, err := s.Pool.Query(ctx, `
  SELECT id, public_ip_id, host(ip_address), event_type, subscription_id, created_at
  FROM user_public_ip_events
  WHERE user_id=$1
  ORDER BY created_at DESC, id DESC
  LIMIT $2
 `, userID, limit)
 if err != nil { return nil, fmt.Errorf("list IP history: %w", err) }
 defer rows.Close()
 events := make([]Event, 0)
 for rows.Next() {
  var event Event
  if err := rows.Scan(&event.ID, &event.PublicIPID, &event.IPAddress, &event.Type, &event.SubscriptionID, &event.CreatedAt); err != nil {
   return nil, fmt.Errorf("scan IP history: %w", err)
  }
  events = append(events, event)
 }
 if err := rows.Err(); err != nil { return nil, fmt.Errorf("iterate IP history: %w", err) }
 return events, nil
}
