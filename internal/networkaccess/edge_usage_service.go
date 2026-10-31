package networkaccess

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// IngestEdgeUsage translates edge session measurements into the existing
// subscription usage ledger, retaining its idempotency and quota updates.
func (s *Service) IngestEdgeUsage(ctx context.Context, edge EdgeUsageReport) (*UsageReportResult, error) {
	if err := edge.Validate(time.Now().UTC()); err != nil {
		return nil, err
	}
	report := UsageReport{
		ReportID: fmt.Sprintf("edge:%s", edge.IdempotencyKey()),
		Source: "edge",
		UploadBytes: int64(edge.UploadedBytes),
		DownloadBytes: int64(edge.DownloadedBytes),
		ReportedAt: edge.EndedAt,
	}
	// PostgreSQL can abort a SERIALIZABLE transaction when concurrent
	// deliveries or entitlement changes race. Retry the entire transaction,
	// never a partially applied ledger mutation.
	for attempt := 0; attempt < 8; attempt++ {
		result, err := s.repository.ApplyEdgeUsageReport(ctx, edge, report)
		if err == nil {
			return result, nil
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || (pgErr.Code != "40001" && pgErr.Code != "40P01") || attempt == 7 {
			return nil, err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, fmt.Errorf("edge usage retry exhausted")
}

