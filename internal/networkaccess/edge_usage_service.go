package networkaccess

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// IngestEdgeUsage translates edge session measurements into the existing
// subscription usage ledger, retaining its idempotency and quota updates.
func (s *Service) IngestEdgeUsage(ctx context.Context, edge EdgeUsageReport) (*UsageReportResult, error) {
	if err := edge.Validate(time.Now().UTC()); err != nil {
		return nil, err
	}
	subscriptionID, err := s.repository.SubscriptionForEdgeInterval(ctx, edge.ClientIP, edge.StartedAt, edge.EndedAt)
	if err != nil {
		return nil, err
	}
	report := UsageReport{
		ReportID: fmt.Sprintf("edge:%s", edge.IdempotencyKey()),
		SubscriptionID: subscriptionID,
		Source: "edge",
		UploadBytes: int64(edge.UploadedBytes),
		DownloadBytes: int64(edge.DownloadedBytes),
		ReportedAt: edge.EndedAt,
	}
	if err := validateReport(report); err != nil {
		return nil, err
	}
	return s.repository.ApplyUsageReport(ctx, report)
}

// SubscriptionForEdgeInterval deliberately rejects ambiguous attribution.
// It uses historical entitlement intervals, not the current owner of an IP.
func (r *Repository) SubscriptionForEdgeInterval(ctx context.Context, clientIP string, start, end time.Time) (uuid.UUID, error) {
	const query = `
		SELECT DISTINCT subscription_id
		FROM network_entitlements
		WHERE client_ip = $1::inet
		  AND activated_at <= $2
		  AND (revoked_at IS NULL OR revoked_at >= $3)
		LIMIT 2
	`
	rows, err := r.db.Query(ctx, query, clientIP, start, end)
	if err != nil {
		return uuid.Nil, fmt.Errorf("resolve edge entitlement: %w", err)
	}
	defer rows.Close()
	var id uuid.UUID
	count := 0
	for rows.Next() {
		var candidate uuid.UUID
		if err := rows.Scan(&candidate); err != nil {
			return uuid.Nil, fmt.Errorf("scan edge entitlement: %w", err)
		}
		id = candidate
		count++
	}
	if err := rows.Err(); err != nil {
		return uuid.Nil, fmt.Errorf("iterate edge entitlements: %w", err)
	}
	if count != 1 {
		return uuid.Nil, ErrSubscriptionNotFound
	}
	return id, nil
}
