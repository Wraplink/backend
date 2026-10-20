package networkentitlement

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

type subscriptionState struct {
	ID             uuid.UUID
	PlanCode       string
	DataLimitBytes int64
	UsedBytes      int64
	StartsAt       time.Time
	ExpiresAt      time.Time
}

func NewRepository(db *pgxpool.Pool) *Repository {
	if db == nil {
		panic("networkentitlement: nil database")
	}

	return &Repository{db: db}
}

func (r *Repository) Activate(
	ctx context.Context,
	userID uuid.UUID,
	clientIP net.IP,
) (*Entitlement, error) {
	if clientIP == nil || clientIP.To4() == nil {
		return nil, ErrInvalidClientIP
	}

	tx, err := r.db.BeginTx(
		ctx,
		pgx.TxOptions{IsoLevel: pgx.Serializable},
	)
	if err != nil {
		return nil, fmt.Errorf("begin entitlement transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	subscription, err := lockUsableSubscription(
		ctx,
		tx,
		userID,
	)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, "UPDATE network_entitlements SET status = 'revoked', revoked_at = now(), updated_at = now() WHERE status = 'active' AND last_activity_at < now() - INTERVAL '30 minutes'"); err != nil {
        return nil, fmt.Errorf("expire idle network access: %w", err)
    }

    existing, err := currentEntitlementForUpdate(
		ctx,
		tx,
		userID,
	)
	if err != nil {
		return nil, err
	}

	clientIPText := clientIP.To4().String()
	if existing != nil &&
		existing.SubscriptionID == subscription.ID &&
		existing.ClientIP == clientIPText {
		if _, err := tx.Exec(ctx, "UPDATE network_entitlements SET last_activity_at = now(), updated_at = now() WHERE id = $1", existing.ID); err != nil {
            return nil, fmt.Errorf("renew connection activity: %w", err)
        }
        existing.Status = "active"
		existing.PlanCode = subscription.PlanCode
		existing.DataLimitBytes = subscription.DataLimitBytes
		existing.UsedBytes = subscription.UsedBytes
		existing.RemainingBytes = remainingBytes(
			subscription.DataLimitBytes,
			subscription.UsedBytes,
		)
		existing.StartsAt = subscription.StartsAt
		existing.ExpiresAt = subscription.ExpiresAt

		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit existing entitlement: %w", err)
		}
		return existing, nil
	}

	if err := ensureClientIPAvailable(
		ctx,
		tx,
		userID,
		clientIPText,
	); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(
		ctx,
		`UPDATE network_entitlements
		 SET status = 'revoked',
		     revoked_at = now(),
		     updated_at = now()
		 WHERE user_id = $1
		   AND status = 'active'`,
		userID,
	); err != nil {
		return nil, fmt.Errorf("revoke previous entitlement: %w", err)
	}

	var result Entitlement
	err = tx.QueryRow(
		ctx,
		`INSERT INTO network_entitlements (
			user_id,
			subscription_id,
			client_ip,
			status
		 ) VALUES ($1, $2, $3::inet, 'active')
		 RETURNING
			id,
			subscription_id,
			host(client_ip),
			status,
			activated_at`,
		userID,
		subscription.ID,
		clientIPText,
	).Scan(
		&result.ID,
		&result.SubscriptionID,
		&result.ClientIP,
		&result.Status,
		&result.ActivatedAt,
	)
	if err != nil {
		if isClientIPConflict(err) {
			return nil, ErrClientIPInUse
		}
		return nil, fmt.Errorf("insert network entitlement: %w", err)
	}

	result.PlanCode = subscription.PlanCode
	result.DataLimitBytes = subscription.DataLimitBytes
	result.UsedBytes = subscription.UsedBytes
	result.RemainingBytes = remainingBytes(
		subscription.DataLimitBytes,
		subscription.UsedBytes,
	)
	result.StartsAt = subscription.StartsAt
	result.ExpiresAt = subscription.ExpiresAt

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit network entitlement: %w", err)
	}

	return &result, nil
}

func (r *Repository) Current(
	ctx context.Context,
	userID uuid.UUID,
) (*Entitlement, error) {
	if _, err := r.db.Exec(ctx, "SELECT promote_user_subscription($1)", userID); err != nil {return nil, fmt.Errorf("reconcile network subscription: %w",err)}
	const query = `
		SELECT
			e.id,
			e.subscription_id,
			host(e.client_ip),
			CASE
				WHEN s.status = 'active'
				 AND s.starts_at <= now()
				 AND s.expires_at > now()
				 AND s.used_bytes < s.data_limit_bytes
                 AND e.last_activity_at >= now() - INTERVAL '30 minutes'
				THEN 'active'
				ELSE 'inactive'
			END,
			s.plan_code,
			COALESCE(m.mode, 'smart'),
			s.data_limit_bytes,
			s.used_bytes,
			GREATEST(s.data_limit_bytes - s.used_bytes, 0),
			s.starts_at,
			s.expires_at,
			e.activated_at
		FROM network_entitlements e
		JOIN user_subscriptions s ON s.id = e.subscription_id
		LEFT JOIN user_connection_modes m ON m.user_id = e.user_id
		WHERE e.user_id = $1
		  AND e.status = 'active'
		ORDER BY e.activated_at DESC
		LIMIT 1
	`

	var result Entitlement
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&result.ID,
		&result.SubscriptionID,
		&result.ClientIP,
		&result.Status,
		&result.PlanCode,
		&result.ConnectionMode,
		&result.DataLimitBytes,
		&result.UsedBytes,
		&result.RemainingBytes,
		&result.StartsAt,
		&result.ExpiresAt,
		&result.ActivatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEntitlementNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get current network entitlement: %w", err)
	}

	return &result, nil
}

func (r *Repository) Deactivate(
	ctx context.Context,
	userID uuid.UUID,
) error {
	result, err := r.db.Exec(
		ctx,
		`UPDATE network_entitlements
		 SET status = 'revoked',
		     revoked_at = now(),
		     updated_at = now()
		 WHERE user_id = $1
		   AND status = 'active'`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("deactivate network entitlement: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrEntitlementNotFound
	}
	return nil
}

// GetMode returns the saved user preference, defaulting to smart for new users.
func (r *Repository) GetMode(ctx context.Context, userID uuid.UUID) (ConnectionMode, error) {
	var mode ConnectionMode
	err := r.db.QueryRow(ctx,
		`SELECT COALESCE((SELECT mode FROM user_connection_modes WHERE user_id = $1), 'smart')`,
		userID,
	).Scan(&mode)
	if err != nil {
		return "", fmt.Errorf("get connection mode: %w", err)
	}
	return mode, nil
}

// SetMode persists a user-scoped preference, regardless of connection state.
func (r *Repository) SetMode(ctx context.Context, userID uuid.UUID, mode ConnectionMode) error {
	if !mode.Valid() {
		return ErrInvalidConnectionMode
	}
	_, err := r.db.Exec(ctx,
		`INSERT INTO user_connection_modes (user_id, mode)
		 VALUES ($1, $2)
		 ON CONFLICT (user_id) DO UPDATE
		   SET mode = EXCLUDED.mode, updated_at = now()`,
		userID, mode,
	)
	if err != nil {
		return fmt.Errorf("save connection mode: %w", err)
	}
	return nil
}

func (r *Repository) Snapshot(
	ctx context.Context,
) ([]SnapshotItem, error) {
	if _, err := r.db.Exec(ctx, "SELECT promote_user_subscription(id) FROM users WHERE EXISTS (SELECT 1 FROM user_subscriptions WHERE user_id=users.id AND status='active' AND (expires_at<=now() OR used_bytes>=data_limit_bytes))"); err != nil {return nil, fmt.Errorf("reconcile network snapshot: %w",err)}
	const query = `
		SELECT
			e.id,
			e.user_id,
			e.subscription_id,
			host(e.client_ip),
			s.plan_code,
			COALESCE(m.mode, 'smart'),
			s.data_limit_bytes,
			s.used_bytes,
			GREATEST(s.data_limit_bytes - s.used_bytes, 0),
			s.expires_at
		FROM network_entitlements e
		JOIN user_subscriptions s ON s.id = e.subscription_id
		LEFT JOIN user_connection_modes m ON m.user_id = e.user_id
		WHERE e.status = 'active'
		  AND s.status = 'active'
		  AND s.starts_at <= now()
		  AND s.expires_at > now()
		  AND s.used_bytes < s.data_limit_bytes
          AND e.last_activity_at >= now() - INTERVAL '30 minutes'
		ORDER BY e.client_ip, e.id
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query network entitlement snapshot: %w", err)
	}
	defer rows.Close()

	items := make([]SnapshotItem, 0)
	for rows.Next() {
		var item SnapshotItem
		if err := rows.Scan(
			&item.EntitlementID,
			&item.UserID,
			&item.SubscriptionID,
			&item.ClientIP,
			&item.PlanCode,
			&item.ConnectionMode,
			&item.DataLimitBytes,
			&item.UsedBytes,
			&item.RemainingBytes,
			&item.ExpiresAt,
		); err != nil {
			return nil, fmt.Errorf("scan network entitlement snapshot: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate network entitlement snapshot: %w", err)
	}

	return items, nil
}

func lockUsableSubscription(
	ctx context.Context,
	tx pgx.Tx,
	userID uuid.UUID,
) (*subscriptionState, error) {
	const query = `
		SELECT
			id,
			plan_code,
			data_limit_bytes,
			used_bytes,
			starts_at,
			expires_at
		FROM user_subscriptions
		WHERE user_id = $1
		  AND status = 'active'
		  AND starts_at <= now()
		  AND expires_at > now()
		  AND used_bytes < data_limit_bytes
		ORDER BY starts_at DESC
		LIMIT 1
		FOR UPDATE
	`

	var state subscriptionState
	err := tx.QueryRow(ctx, query, userID).Scan(
		&state.ID,
		&state.PlanCode,
		&state.DataLimitBytes,
		&state.UsedBytes,
		&state.StartsAt,
		&state.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoUsableSubscription
	}
	if err != nil {
		return nil, fmt.Errorf("lock usable subscription: %w", err)
	}

	return &state, nil
}

func currentEntitlementForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	userID uuid.UUID,
) (*Entitlement, error) {
	var result Entitlement
	err := tx.QueryRow(
		ctx,
		`SELECT
			id,
			subscription_id,
			host(client_ip),
			status,
			activated_at
		 FROM network_entitlements
		 WHERE user_id = $1
		   AND status = 'active'
		 LIMIT 1
		 FOR UPDATE`,
		userID,
	).Scan(
		&result.ID,
		&result.SubscriptionID,
		&result.ClientIP,
		&result.Status,
		&result.ActivatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lock current entitlement: %w", err)
	}
	return &result, nil
}

func ensureClientIPAvailable(
	ctx context.Context,
	tx pgx.Tx,
	userID uuid.UUID,
	clientIP string,
) error {
	var ownerID uuid.UUID
	err := tx.QueryRow(
		ctx,
		`SELECT user_id
		 FROM network_entitlements
		 WHERE client_ip = $1::inet
		   AND status = 'active'
		 LIMIT 1
		 FOR UPDATE`,
		clientIP,
	).Scan(&ownerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check network entitlement client IP: %w", err)
	}
	if ownerID != userID {
		return ErrClientIPInUse
	}
	return nil
}

func remainingBytes(limit, used int64) int64 {
	remaining := limit - used
	if remaining < 0 {
		return 0
	}
	return remaining
}

func isClientIPConflict(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "23505" &&
		pgErr.ConstraintName == "uq_network_entitlements_client_ip_active"
}
