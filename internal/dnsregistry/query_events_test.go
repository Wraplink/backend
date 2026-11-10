package dnsregistry

import "testing"

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
