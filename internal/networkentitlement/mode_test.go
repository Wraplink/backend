package networkentitlement

import "testing"

func TestConnectionModeValid(t *testing.T) {
	for _, mode := range []ConnectionMode{ConnectionModeSmart, ConnectionModeGaming, ConnectionModeStreaming} {
		if !mode.Valid() {
			t.Fatalf("expected mode %q to be valid", mode)
		}
	}
	for _, mode := range []ConnectionMode{"", "invalid", "GAMING", "fast"} {
		if mode.Valid() {
			t.Fatalf("expected mode %q to be invalid", mode)
		}
	}
}
