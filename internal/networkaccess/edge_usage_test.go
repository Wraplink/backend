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
	report.Sequence = 0
	if err := report.Validate(now); err == nil {
		t.Fatal("expected invalid sequence")
	}
}
