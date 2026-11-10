package dnsregistry

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidQueryDomainAcceptsDNSServiceLabels(t *testing.T) {
	for _, domain := range []string{
		"example.org",
		"_sip._tcp.example.org",
		"_dmarc.example.org",
		"_acme-challenge.example.org",
	} {
		if !validQueryDomain(domain) {
			t.Errorf("valid DNS query domain rejected: %q", domain)
		}
	}
}

func TestValidQueryDomainRejectsMalformedNames(t *testing.T) {
	for _, domain := range []string{
		"",
		".",
		"example..org",
		"example.org.",
		"bad domain.example",
		"UPPER.example",
	} {
		if validQueryDomain(domain) {
			t.Errorf("invalid DNS query domain accepted: %q", domain)
		}
	}
}

func TestDNSQueryBatchAcceptsDNSControlWireFormat(t *testing.T) {
	// Matches the DNS-Control reporter's outbound JSON field names.
	const payload = `{
		"controller_id": "dns-eu-01",
		"events": [{
			"event_id": "d19d4a31-3eb4-4a87-bad0-82b4cfcf3d90",
			"observed_at": "2026-11-10T10:15:00Z",
			"domain": "_sip._tcp.example.org",
			"qtype": "SRV",
			"rcode": "NOERROR",
			"resolution_mode": "normal",
			"duration_ms": 12
		}]
	}`
	var batch DNSQueryBatch
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&batch); err != nil {
		t.Fatalf("DNS-Control wire payload rejected: %v", err)
	}
	if batch.ControllerID != "dns-eu-01" || len(batch.Events) != 1 {
		t.Fatalf("unexpected batch: %+v", batch)
	}
	event := batch.Events[0]
	if event.EventID.String() != "d19d4a31-3eb4-4a87-bad0-82b4cfcf3d90" ||
		!validQueryDomain(event.Domain) || event.QType != "SRV" ||
		event.RCode != "NOERROR" || event.ResolutionMode != "normal" ||
		event.DurationMS != 12 || event.ObservedAt.IsZero() {
		t.Fatalf("unexpected decoded DNS-Control event: %+v", event)
	}
}
