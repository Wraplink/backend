package networkaccess

import (
	"context"
	"fmt"
	"time"

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
	if err := validateReport(report); err != nil {
		return nil, err
	}
	return s.repository.ApplyEdgeUsageReport(ctx, edge, report)
}

