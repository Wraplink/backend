package networkaccess

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	AccessStatusActive         = "active"
	AccessStatusQuotaExhausted = "quota_exhausted"
	AccessStatusExpired        = "expired"
	AccessStatusUnavailable    = "unavailable"
)

var (
	ErrNoActiveSubscription  = errors.New("no active subscription")
	ErrSubscriptionNotFound  = errors.New("subscription not found")
	ErrSubscriptionNotUsable = errors.New("subscription is not usable for traffic accounting")
	ErrInvalidReport         = errors.New("invalid usage report")
	ErrReportConflict        = errors.New("usage report ID conflict")
	ErrUsageIngestDisabled   = errors.New("usage ingest is disabled")
)

type ProxyConnection struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

type ConnectionDetails struct {
	DNSServers []string         `json:"dnsServers"`
	Proxy      *ProxyConnection `json:"proxy,omitempty"`
}

type AccessInfo struct {
	SubscriptionID uuid.UUID          `json:"subscriptionId"`
	PlanCode       string             `json:"planCode"`
	Status         string             `json:"status"`
	DataLimitBytes int64              `json:"dataLimitBytes"`
	UsedBytes      int64              `json:"usedBytes"`
	RemainingBytes int64              `json:"remainingBytes"`
	StartsAt       time.Time          `json:"startsAt"`
	ExpiresAt      time.Time          `json:"expiresAt"`
	Connection     *ConnectionDetails `json:"connection,omitempty"`
}

type DailyUsage struct {
	Date          string `json:"date"`
	UploadBytes   int64  `json:"uploadBytes"`
	DownloadBytes int64  `json:"downloadBytes"`
	TotalBytes    int64  `json:"totalBytes"`
}

type UsageSummary struct {
	SubscriptionID uuid.UUID    `json:"subscriptionId"`
	PlanCode       string       `json:"planCode"`
	Status         string       `json:"status"`
	DataLimitBytes int64        `json:"dataLimitBytes"`
	UsedBytes      int64        `json:"usedBytes"`
	RemainingBytes int64        `json:"remainingBytes"`
	PercentUsed    float64      `json:"percentUsed"`
	StartsAt       time.Time    `json:"startsAt"`
	ExpiresAt      time.Time    `json:"expiresAt"`
	Daily          []DailyUsage `json:"daily"`
}

type UsageReport struct {
	ReportID       string    `json:"reportId"`
	SubscriptionID uuid.UUID `json:"subscriptionId"`
	Source         string    `json:"source"`
	UploadBytes    int64     `json:"uploadBytes"`
	DownloadBytes  int64     `json:"downloadBytes"`
	ReportedAt     time.Time `json:"reportedAt"`
}

type UsageReportResult struct {
	ReportID       string    `json:"reportId"`
	SubscriptionID uuid.UUID `json:"subscriptionId"`
	Applied        bool      `json:"applied"`
	Status         string    `json:"status"`
	DataLimitBytes int64     `json:"dataLimitBytes"`
	UsedBytes      int64     `json:"usedBytes"`
	RemainingBytes int64     `json:"remainingBytes"`
}

type IngestRequest struct {
	Reports []UsageReport `json:"reports"`
}

type IngestResponse struct {
	Results []UsageReportResult `json:"results"`
}
