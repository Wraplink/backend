package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	StatusPendingPayment = "pending_payment"
	StatusActive         = "active"
	StatusExpired        = "expired"
	StatusCancelled      = "cancelled"
)

type Subscription struct {
	ID             uuid.UUID  `json:"id"`
	UserID         uuid.UUID  `json:"userId"`
	PlanID         uuid.UUID  `json:"planId"`
	PlanCode       string     `json:"planCode"`
	PlanName       string     `json:"planName"`
	PriceMinor     int64      `json:"priceMinor"`
	Currency       string     `json:"currency"`
	DurationDays   int        `json:"durationDays"`
	DataLimitBytes int64      `json:"dataLimitBytes"`
	UsedBytes      int64      `json:"usedBytes"`
	RemainingBytes int64      `json:"remainingBytes"`
	Status         string     `json:"status"`
	StartsAt       *time.Time `json:"startsAt,omitempty"`
	ExpiresAt      *time.Time `json:"expiresAt,omitempty"`
	ActivatedAt    *time.Time `json:"activatedAt,omitempty"`
	CancelledAt    *time.Time `json:"cancelledAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type CreatePendingParams struct {
	UserID         uuid.UUID
	PlanID         uuid.UUID
	PlanCode       string
	PlanName       string
	PriceMinor     int64
	Currency       string
	DurationDays   int
	DataLimitBytes int64
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	if db == nil {
		panic("subscriptions: nil database")
	}

	return &Repository{db: db}
}

func (r *Repository) CreatePending(
	ctx context.Context,
	params CreatePendingParams,
) (*Subscription, error) {
	const query = `
		INSERT INTO user_subscriptions (
			user_id,
			plan_id,
			plan_code,
			plan_name,
			price_minor,
			currency,
			duration_days,
			data_limit_bytes,
			status
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING
			id,
			user_id,
			plan_id,
			plan_code,
			plan_name,
			price_minor,
			currency,
			duration_days,
			data_limit_bytes,
			used_bytes,
			GREATEST(data_limit_bytes - used_bytes, 0),
			status,
			starts_at,
			expires_at,
			activated_at,
			cancelled_at,
			created_at,
			updated_at
	`

	var result Subscription

	err := r.db.QueryRow(
		ctx,
		query,
		params.UserID,
		params.PlanID,
		params.PlanCode,
		params.PlanName,
		params.PriceMinor,
		params.Currency,
		params.DurationDays,
		params.DataLimitBytes,
		StatusPendingPayment,
	).Scan(
		&result.ID,
		&result.UserID,
		&result.PlanID,
		&result.PlanCode,
		&result.PlanName,
		&result.PriceMinor,
		&result.Currency,
		&result.DurationDays,
		&result.DataLimitBytes,
		&result.UsedBytes,
		&result.RemainingBytes,
		&result.Status,
		&result.StartsAt,
		&result.ExpiresAt,
		&result.ActivatedAt,
		&result.CancelledAt,
		&result.CreatedAt,
		&result.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create pending subscription: %w", err)
	}

	return &result, nil
}

func (r *Repository) ListByUser(
	ctx context.Context,
	userID uuid.UUID,
) ([]Subscription, error) {
	const query = `
		SELECT
			id,
			user_id,
			plan_id,
			plan_code,
			plan_name,
			price_minor,
			currency,
			duration_days,
			data_limit_bytes,
			used_bytes,
			GREATEST(data_limit_bytes - used_bytes, 0),
			status,
			starts_at,
			expires_at,
			activated_at,
			cancelled_at,
			created_at,
			updated_at
		FROM user_subscriptions
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	defer rows.Close()

	result := make([]Subscription, 0)

	for rows.Next() {
		var item Subscription

		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.PlanID,
			&item.PlanCode,
			&item.PlanName,
			&item.PriceMinor,
			&item.Currency,
			&item.DurationDays,
			&item.DataLimitBytes,
			&item.UsedBytes,
			&item.RemainingBytes,
			&item.Status,
			&item.StartsAt,
			&item.ExpiresAt,
			&item.ActivatedAt,
			&item.CancelledAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan subscription: %w", err)
		}

		result = append(result, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subscriptions: %w", err)
	}

	return result, nil
}

func (r *Repository) Current(
	ctx context.Context,
	userID uuid.UUID,
) (*Subscription, error) {
	const query = `
		SELECT
			id,
			user_id,
			plan_id,
			plan_code,
			plan_name,
			price_minor,
			currency,
			duration_days,
			data_limit_bytes,
			used_bytes,
			GREATEST(data_limit_bytes - used_bytes, 0),
			status,
			starts_at,
			expires_at,
			activated_at,
			cancelled_at,
			created_at,
			updated_at
		FROM user_subscriptions
		WHERE user_id = $1
		  AND status = $2
		  AND starts_at <= now()
		  AND expires_at > now()
		ORDER BY starts_at DESC
		LIMIT 1
	`

	var result Subscription

	err := r.db.QueryRow(ctx, query, userID, StatusActive).Scan(
		&result.ID,
		&result.UserID,
		&result.PlanID,
		&result.PlanCode,
		&result.PlanName,
		&result.PriceMinor,
		&result.Currency,
		&result.DurationDays,
		&result.DataLimitBytes,
		&result.UsedBytes,
		&result.RemainingBytes,
		&result.Status,
		&result.StartsAt,
		&result.ExpiresAt,
		&result.ActivatedAt,
		&result.CancelledAt,
		&result.CreatedAt,
		&result.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get current subscription: %w", err)
	}

	return &result, nil
}

func (r *Repository) CancelPending(
	ctx context.Context,
	userID uuid.UUID,
	subscriptionID uuid.UUID,
) (bool, error) {
	const query = `
		UPDATE user_subscriptions
		SET
			status = $3,
			cancelled_at = now(),
			updated_at = now()
		WHERE id = $1
		  AND user_id = $2
		  AND status = $4
          AND NOT EXISTS (SELECT 1 FROM card_transfer_receipts WHERE subscription_id = $1 AND status = 'pending_review')
	`

	result, err := r.db.Exec(
		ctx,
		query,
		subscriptionID,
		userID,
		StatusCancelled,
		StatusPendingPayment,
	)
	if err != nil {
		return false, fmt.Errorf("cancel pending subscription: %w", err)
	}

	return result.RowsAffected() == 1, nil
}

// AddUsage is intentionally not exposed through HTTP. The accounting/edge
// collector can call this method later when traffic deltas are reported.
func (r *Repository) AddUsage(
	ctx context.Context,
	subscriptionID uuid.UUID,
	bytes int64,
) error {
	if bytes <= 0 {
		return nil
	}

	const query = `
		UPDATE user_subscriptions
		SET
			used_bytes = used_bytes + $2,
			updated_at = now()
		WHERE id = $1
		  AND status = $3
	`

	_, err := r.db.Exec(ctx, query, subscriptionID, bytes, StatusActive)
	if err != nil {
		return fmt.Errorf("add subscription usage: %w", err)
	}

	return nil
}
