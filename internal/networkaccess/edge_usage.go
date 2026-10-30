package networkaccess

import (
	"errors"
	"fmt"
	"math"
	"net"
	"strings"
	"time"
)

var ErrInvalidEdgeUsage = errors.New("invalid edge usage report")

// EdgeUsageReport is the wire format sent by the edge agent.
// Sequence is unique within an agent's lifetime, including restarts.
type EdgeUsageReport struct {
	AgentID string `json:"agentId"`
	Sequence uint64 `json:"sequence"`
	ClientIP string `json:"clientIp"`
	UploadedBytes uint64 `json:"uploadedBytes"`
	DownloadedBytes uint64 `json:"downloadedBytes"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt time.Time `json:"endedAt"`
}

func (r EdgeUsageReport) Validate(now time.Time) error {
	if strings.TrimSpace(r.AgentID) == "" || strings.TrimSpace(r.AgentID) != r.AgentID || len(r.AgentID) > 80 || r.Sequence == 0 {
		return ErrInvalidEdgeUsage
	}
	ip := net.ParseIP(r.ClientIP)
	if ip == nil || ip.To4() == nil || ip.String() != r.ClientIP {
		return ErrInvalidEdgeUsage
	}
	if r.UploadedBytes > math.MaxInt64 || r.DownloadedBytes > math.MaxInt64 ||
		r.UploadedBytes > math.MaxInt64-r.DownloadedBytes ||
		r.UploadedBytes+r.DownloadedBytes == 0 {
		return ErrInvalidEdgeUsage
	}
	if r.StartedAt.IsZero() || r.EndedAt.IsZero() || r.StartedAt.After(r.EndedAt) ||
		r.EndedAt.After(now.Add(10*time.Minute)) ||
		r.StartedAt.Before(now.Add(-24*time.Hour)) {
		return ErrInvalidEdgeUsage
	}
	return nil
}

func (r EdgeUsageReport) IdempotencyKey() string {
	return fmt.Sprintf("%s:%d", r.AgentID, r.Sequence)
}
