package networkaccess

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultUsageDays = 30
	maxUsageDays     = 90
	maxBatchSize     = 500
)

type Service struct {
	repository *Repository
	connection ConnectionDetails
}

func NewService(repository *Repository, connection ConnectionDetails) *Service {
	if repository == nil {
		panic("networkaccess: nil repository")
	}

	return &Service{
		repository: repository,
		connection: connection,
	}
}

func (s *Service) Access(
	ctx context.Context,
	userID uuid.UUID,
) (*AccessInfo, error) {
	state, err := s.repository.CurrentAccess(ctx, userID)
	if err != nil {
		return nil, err
	}

	remaining := state.DataLimitBytes - state.UsedBytes
	if remaining < 0 {
		remaining = 0
	}

	result := &AccessInfo{
		SubscriptionID: state.ID,
		PlanCode:       state.PlanCode,
		Status:         accessStatus(state, time.Now()),
		DataLimitBytes: state.DataLimitBytes,
		UsedBytes:      state.UsedBytes,
		RemainingBytes: remaining,
		StartsAt:       *state.StartsAt,
		ExpiresAt:      *state.ExpiresAt,
	}

	if len(s.connection.DNSServers) > 0 || s.connection.Proxy != nil {
		connection := s.connection
		result.Connection = &connection
	}

	return result, nil
}

func (s *Service) Usage(
	ctx context.Context,
	userID uuid.UUID,
	days int,
) (*UsageSummary, error) {
	state, err := s.repository.CurrentAccess(ctx, userID)
	if err != nil {
		return nil, err
	}

	days = normalizeUsageDays(days)
	daily, err := s.repository.DailyUsage(ctx, state.ID, days)
	if err != nil {
		return nil, err
	}

	remaining := state.DataLimitBytes - state.UsedBytes
	if remaining < 0 {
		remaining = 0
	}

	percent := 0.0
	if state.DataLimitBytes > 0 {
		percent = (float64(state.UsedBytes) / float64(state.DataLimitBytes)) * 100
	}

	return &UsageSummary{
		SubscriptionID: state.ID,
		PlanCode:       state.PlanCode,
		Status:         accessStatus(state, time.Now()),
		DataLimitBytes: state.DataLimitBytes,
		UsedBytes:      state.UsedBytes,
		RemainingBytes: remaining,
		PercentUsed:    percent,
		StartsAt:       *state.StartsAt,
		ExpiresAt:      *state.ExpiresAt,
		Daily:          daily,
	}, nil
}

func (s *Service) Ingest(
	ctx context.Context,
	request IngestRequest,
) (*IngestResponse, error) {
	if len(request.Reports) == 0 || len(request.Reports) > maxBatchSize {
		return nil, ErrInvalidReport
	}

	results := make([]UsageReportResult, 0, len(request.Reports))
	for _, report := range request.Reports {
		report = normalizeReport(report)
		if err := validateReport(report); err != nil {
			return nil, err
		}

		result, err := s.repository.ApplyUsageReport(ctx, report)
		if err != nil {
			return nil, err
		}
		results = append(results, *result)
	}

	return &IngestResponse{Results: results}, nil
}

func normalizeReport(report UsageReport) UsageReport {
	report.ReportID = strings.TrimSpace(report.ReportID)
	report.Source = strings.TrimSpace(report.Source)
	if report.Source == "" {
		report.Source = "edge"
	}
	if report.ReportedAt.IsZero() {
		report.ReportedAt = time.Now().UTC()
	}
	return report
}

func validateReport(report UsageReport) error {
	if report.ReportID == "" || len(report.ReportID) > 160 {
		return ErrInvalidReport
	}
	if report.SubscriptionID == uuid.Nil {
		return ErrInvalidReport
	}
	if report.Source == "" || len(report.Source) > 80 {
		return ErrInvalidReport
	}
	if report.UploadBytes < 0 || report.DownloadBytes < 0 {
		return ErrInvalidReport
	}
	if report.UploadBytes == 0 && report.DownloadBytes == 0 {
		return ErrInvalidReport
	}
	if report.UploadBytes > math.MaxInt64-report.DownloadBytes {
		return ErrInvalidReport
	}
	if report.ReportedAt.After(time.Now().Add(10 * time.Minute)) {
		return ErrInvalidReport
	}
	return nil
}

func normalizeUsageDays(days int) int {
	if days <= 0 {
		return defaultUsageDays
	}
	if days > maxUsageDays {
		return maxUsageDays
	}
	return days
}
