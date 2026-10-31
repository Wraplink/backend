package networkaccess

import (
	"testing"
	"time"
)

func TestEdgeUsageValidation(t *testing.T) {
	now := time.Now().UTC()
	report := EdgeUsageReport{
		AgentID: "edge-eu-1",
		Sequence: 1,
		ClientIP: "198.51.100.10",
		UploadedBytes: 10,
		DownloadedBytes: 20,
		StartedAt: now.Add(-time.Minute),
		EndedAt: now,
	}
	if err := report.Validate(now); err != nil {
		t.Fatal(err)
	}
	report.ClientIP = "invalid"
	if err := report.Validate(now); err == nil { t.Fatal("expected invalid IP") }
	report.ClientIP = "198.51.100.10"
	report.Sequence = 0
	if err := report.Validate(now); err == nil {
		t.Fatal("expected invalid sequence")
	}
}

func TestEdgeUsageRejectsInvalidIntervalsAndOverflow(t *testing.T) {
	now := time.Now().UTC()
	base := EdgeUsageReport{
		AgentID: "edge-eu-1", Sequence: 42, ClientIP: "198.51.100.10",
		UploadedBytes: 10, DownloadedBytes: 20,
		StartedAt: now.Add(-time.Minute), EndedAt: now,
	}
	tests := []struct {
		name string
		change func(*EdgeUsageReport)
	}{
		{"agent whitespace", func(r *EdgeUsageReport) { r.AgentID = " edge-eu-1" }},
		{"zero bytes", func(r *EdgeUsageReport) { r.UploadedBytes = 0; r.DownloadedBytes = 0 }},
		{"reversed timestamps", func(r *EdgeUsageReport) { r.StartedAt = now.Add(time.Minute) }},
		{"older than 24h", func(r *EdgeUsageReport) { r.StartedAt = now.Add(-25 * time.Hour) }},
		{"future completion", func(r *EdgeUsageReport) { r.EndedAt = now.Add(11 * time.Minute) }},
		{"overflow", func(r *EdgeUsageReport) { r.UploadedBytes = ^uint64(0) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			report := base
			tc.change(&report)
			if err := report.Validate(now); err == nil { t.Fatal("expected invalid edge usage") }
		})
	}
}
