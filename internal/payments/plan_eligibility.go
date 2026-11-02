package payments

import (
 "context"
 "errors"
 "fmt"
 "time"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)

var ErrPlanUnavailable = errors.New("plan is disabled or unpublished")

// planPayable is deliberately checked at payment time, not just at order creation.
func planPayable(active bool, publishAt *time.Time, now time.Time) bool {
 return active && (publishAt == nil || !publishAt.After(now))
}

// requirePayablePlan must run in the payment/review transaction. The shared
// plan row lock serializes eligibility checks with administrator plan updates.
func requirePayablePlan(ctx context.Context, tx pgx.Tx, subscriptionID uuid.UUID) error {
 var active bool
 var publishAt *time.Time
 err := tx.QueryRow(ctx,`SELECT p.active, p.publish_at
  FROM user_subscriptions s
  JOIN plans p ON p.id = s.plan_id
  WHERE s.id = $1
  FOR SHARE OF p`, subscriptionID).Scan(&active,&publishAt)
 if errors.Is(err,pgx.ErrNoRows) {return ErrPlanUnavailable}
 if err!=nil {return fmt.Errorf("check plan payment eligibility: %w",err)}
 if !planPayable(active,publishAt,time.Now()) {return ErrPlanUnavailable}
 return nil
}
