package networkaccess

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

type subscriptionState struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	PlanCode       string
	Status         string
	DataLimitBytes int64
	UsedBytes      int64
	StartsAt       *time.Time
	ExpiresAt      *time.Time
}

func NewRepository(db *pgxpool.Pool) *Repository {
	if db == nil {
		panic("networkaccess: nil database")
	}

	return &Repository{db: db}
}

func (r *Repository) CurrentAccess(
	ctx context.Context,
	userID uuid.UUID,
) (*subscriptionState, error) {
	const query = `
		SELECT
			id,
			user_id,
			plan_code,
			status,
			data_limit_bytes,
			used_bytes,
			starts_at,
			expires_at
		FROM user_subscriptions
		WHERE user_id = $1
		  AND status = 'active'
		  AND starts_at <= now()
		  AND expires_at > now()
		ORDER BY starts_at DESC
		LIMIT 1
	`

	var state subscriptionState
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&state.ID,
		&state.UserID,
		&state.PlanCode,
		&state.Status,
		&state.DataLimitBytes,
		&state.UsedBytes,
		&state.StartsAt,
		&state.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoActiveSubscription
	}
	if err != nil {
		return nil, fmt.Errorf("get current network access: %w", err)
	}

	return &state, nil
}

func (r *Repository) DailyUsage(
	ctx context.Context,
	subscriptionID uuid.UUID,
	days int,
) ([]DailyUsage, error) {
	const query = `
		SELECT
			to_char((reported_at AT TIME ZONE 'UTC')::date, 'YYYY-MM-DD'),
			COALESCE(SUM(upload_bytes), 0),
			COALESCE(SUM(download_bytes), 0)
		FROM traffic_usage_reports
		WHERE subscription_id = $1
		  AND reported_at >= now() - (($2::integer - 1) * INTERVAL '1 day')
		GROUP BY (reported_at AT TIME ZONE 'UTC')::date
		ORDER BY (reported_at AT TIME ZONE 'UTC')::date DESC
	`

	rows, err := r.db.Query(ctx, query, subscriptionID, days)
	if err != nil {
		return nil, fmt.Errorf("list daily usage: %w", err)
	}
	defer rows.Close()

	result := make([]DailyUsage, 0)
	for rows.Next() {
		var item DailyUsage
		if err := rows.Scan(
			&item.Date,
			&item.UploadBytes,
			&item.DownloadBytes,
		); err != nil {
			return nil, fmt.Errorf("scan daily usage: %w", err)
		}
		item.TotalBytes = item.UploadBytes + item.DownloadBytes
		result = append(result, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate daily usage: %w", err)
	}

	return result, nil
}

func (r *Repository) ApplyUsageReport(
	ctx context.Context,
	report UsageReport,
) (*UsageReportResult, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin usage report transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	state, err := lockSubscription(ctx, tx, report.SubscriptionID)
	if err != nil {
		return nil, err
	}
	if state.Status != "active" && state.Status != "expired" {
		return nil, ErrSubscriptionNotUsable
	}
	if state.StartsAt == nil || state.ExpiresAt == nil {
		return nil, ErrSubscriptionNotUsable
	}

	result, err := tx.Exec(
		ctx,
		`INSERT INTO traffic_usage_reports (
			report_id,
			subscription_id,
			user_id,
			source,
			upload_bytes,
			download_bytes,
			reported_at
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (report_id) DO NOTHING`,
		report.ReportID,
		report.SubscriptionID,
		state.UserID,
		report.Source,
		report.UploadBytes,
		report.DownloadBytes,
		report.ReportedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert usage report: %w", err)
	}

	applied := result.RowsAffected() == 1
	if !applied {
		matches, err := usageReportMatches(ctx, tx, report)
		if err != nil {
			return nil, err
		}
		if !matches {
			return nil, ErrReportConflict
		}
	}

	if applied {
		totalBytes := report.UploadBytes + report.DownloadBytes
		err = tx.QueryRow(
			ctx,
			`UPDATE user_subscriptions
			 SET used_bytes = used_bytes + $2,
			     status = CASE
			         WHEN status = 'active' AND expires_at <= now() THEN 'expired'
			         ELSE status
			     END,
			     updated_at = now()
			 WHERE id = $1
			 RETURNING status, used_bytes`,
			report.SubscriptionID,
			totalBytes,
		).Scan(&state.Status, &state.UsedBytes)
		if err != nil {
			return nil, fmt.Errorf("update subscription usage: %w", err)
		}
	}

	response := usageReportResult(report.ReportID, state, applied, time.Now())

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit usage report transaction: %w", err)
	}

	return response, nil
}

func lockSubscription(
	ctx context.Context,
	tx pgx.Tx,
	subscriptionID uuid.UUID,
) (*subscriptionState, error) {
	const query = `
		SELECT
			id,
			user_id,
			plan_code,
			status,
			data_limit_bytes,
			used_bytes,
			starts_at,
			expires_at
		FROM user_subscriptions
		WHERE id = $1
		FOR UPDATE
	`

	var state subscriptionState
	err := tx.QueryRow(ctx, query, subscriptionID).Scan(
		&state.ID,
		&state.UserID,
		&state.PlanCode,
		&state.Status,
		&state.DataLimitBytes,
		&state.UsedBytes,
		&state.StartsAt,
		&state.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSubscriptionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock subscription for usage: %w", err)
	}

	return &state, nil
}

func usageReportMatches(
	ctx context.Context,
	tx pgx.Tx,
	report UsageReport,
) (bool, error) {
	var subscriptionID uuid.UUID
	var source string
	var uploadBytes int64
	var downloadBytes int64

	err := tx.QueryRow(
		ctx,
		`SELECT subscription_id, source, upload_bytes, download_bytes
		 FROM traffic_usage_reports
		 WHERE report_id = $1`,
		report.ReportID,
	).Scan(&subscriptionID, &source, &uploadBytes, &downloadBytes)
	if err != nil {
		return false, fmt.Errorf("load existing usage report: %w", err)
	}

	return subscriptionID == report.SubscriptionID &&
		source == report.Source &&
		uploadBytes == report.UploadBytes &&
		downloadBytes == report.DownloadBytes, nil
}

func usageReportResult(
	reportID string,
	state *subscriptionState,
	applied bool,
	now time.Time,
) *UsageReportResult {
	remaining := state.DataLimitBytes - state.UsedBytes
	if remaining < 0 {
		remaining = 0
	}

	return &UsageReportResult{
		ReportID:       reportID,
		SubscriptionID: state.ID,
		Applied:        applied,
		Status:         accessStatus(state, now),
		DataLimitBytes: state.DataLimitBytes,
		UsedBytes:      state.UsedBytes,
		RemainingBytes: remaining,
	}
}

func accessStatus(state *subscriptionState, now time.Time) string {
	if state == nil || state.StartsAt == nil || state.ExpiresAt == nil {
		return AccessStatusUnavailable
	}
	if state.Status != "active" || !state.ExpiresAt.After(now) {
		return AccessStatusExpired
	}
	if state.UsedBytes >= state.DataLimitBytes {
		return AccessStatusQuotaExhausted
	}
	return AccessStatusActive
}
