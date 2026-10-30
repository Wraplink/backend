package networkaccess

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ApplyEdgeUsageReport serializes attribution with subscription accounting.
// Historical entitlement intervals are checked before any charge is applied.
func (r *Repository) ApplyEdgeUsageReport(ctx context.Context, edge EdgeUsageReport, report UsageReport) (*UsageReportResult, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil { return nil, fmt.Errorf("begin edge usage transaction: %w", err) }
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialize concurrent reports for the same IP. Entitlement writers must
	// also use this lock to eliminate assignment races.
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", edge.ClientIP); err != nil {
		return nil, fmt.Errorf("lock edge client: %w", err)
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT subscription_id
		FROM network_entitlements
		WHERE client_ip = $1::inet
		  AND activated_at <= $2
		  AND (revoked_at IS NULL OR revoked_at >= $3)
		LIMIT 2`, edge.ClientIP, edge.StartedAt, edge.EndedAt)
	if err != nil { return nil, fmt.Errorf("resolve edge entitlement: %w", err) }
	var subscriptionID uuid.UUID
	count := 0
	for rows.Next() {
		if err := rows.Scan(&subscriptionID); err != nil { rows.Close(); return nil, err }
		count++
	}
	err = rows.Err()
	rows.Close()
	if err != nil { return nil, err }
	if count != 1 { return nil, ErrSubscriptionNotFound }
	report.SubscriptionID = subscriptionID
	if err := validateReport(report); err != nil { return nil, err }

	state, err := lockSubscription(ctx, tx, subscriptionID)
	if err != nil { return nil, err }
	if state.Status != "active" && state.Status != "expired" { return nil, ErrSubscriptionNotUsable }
	if state.StartsAt == nil || state.ExpiresAt == nil { return nil, ErrSubscriptionNotUsable }
	tag, err := tx.Exec(ctx, `
		INSERT INTO traffic_usage_reports
		(report_id, subscription_id, user_id, source, upload_bytes, download_bytes, reported_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (report_id) DO NOTHING`,
		report.ReportID, subscriptionID, state.UserID, report.Source,
		report.UploadBytes, report.DownloadBytes, report.ReportedAt)
	if err != nil { return nil, fmt.Errorf("insert edge report: %w", err) }
	applied := tag.RowsAffected() == 1
	if !applied {
		matches, err := usageReportMatches(ctx, tx, report)
		if err != nil { return nil, err }
		if !matches { return nil, ErrReportConflict }
	} else {
		total := report.UploadBytes + report.DownloadBytes
		err = tx.QueryRow(ctx, `
			UPDATE user_subscriptions
			SET used_bytes = used_bytes + $2, updated_at = now()
			WHERE id = $1 AND used_bytes <= 9223372036854775807 - $2
			RETURNING status, used_bytes`, subscriptionID, total).Scan(&state.Status, &state.UsedBytes)
		if errors.Is(err, pgx.ErrNoRows) { return nil, ErrInvalidReport }
		if err != nil { return nil, fmt.Errorf("update subscription usage: %w", err) }
		if _, err := tx.Exec(ctx, `
			UPDATE network_entitlements
			SET last_activity_at = GREATEST(last_activity_at, $2), updated_at = now()
			WHERE subscription_id = $1 AND status = 'active'
			AND last_activity_at >= now() - INTERVAL '30 minutes'`,
			subscriptionID, report.ReportedAt); err != nil {
			return nil, fmt.Errorf("record edge activity: %w", err)
		}
	}
	response := usageReportResult(report.ReportID, state, applied, time.Now())
	if err := tx.Commit(ctx); err != nil { return nil, fmt.Errorf("commit edge usage: %w", err) }
	return response, nil
}
