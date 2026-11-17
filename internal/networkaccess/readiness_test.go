package networkaccess

import "testing"

func TestAssessConnectionReadiness(t *testing.T) {
 tests := []struct {
  name, status string
  dns, accessReady, dnsReady bool
  reason string
 }{
  {"inactive", AccessStatusExpired, true, false, true, "ACCESS_NOT_ACTIVE"},
  {"missing dns", AccessStatusActive, false, true, false, "DNS_CONTROL_UNAVAILABLE"},
  {"control plane ready", AccessStatusActive, true, true, true, "CLIENT_CONNECTIVITY_UNVERIFIED"},
 }
 for _, tt := range tests {
  t.Run(tt.name, func(t *testing.T) {
   got := assessConnectionReadiness(tt.status, tt.dns)
   if got.AccessReady != tt.accessReady || got.DNSReady != tt.dnsReady || got.Reason != tt.reason || got.ClientVerified {
    t.Fatalf("unexpected readiness: %+v", got)
   }
  })
 }
}
