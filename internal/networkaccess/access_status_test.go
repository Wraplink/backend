package networkaccess

import (
	"testing"
	"time"
)

func TestAccessStatusUnlimitedBandwidth(t *testing.T) {
	now := time.Now().UTC()
	starts := now.Add(-time.Hour)
	expires := now.Add(time.Hour)
	state := &subscriptionState{
		Status: "active", DataLimitBytes: 0, UsedBytes: 1048576,
		StartsAt: &starts, ExpiresAt: &expires,
	}
	if got := accessStatus(state, now); got != AccessStatusActive {
		t.Fatalf("unlimited plan must remain active, got %q", got)
	}
}

func TestAccessStatusExhaustedLimitedBandwidth(t *testing.T) {
	now := time.Now().UTC()
	starts := now.Add(-time.Hour)
	expires := now.Add(time.Hour)
	state := &subscriptionState{
		Status: "active", DataLimitBytes: 1024, UsedBytes: 1024,
		StartsAt: &starts, ExpiresAt: &expires,
	}
	if got := accessStatus(state, now); got != AccessStatusQuotaExhausted {
		t.Fatalf("exhausted limited plan must be rejected, got %q", got)
	}
}
