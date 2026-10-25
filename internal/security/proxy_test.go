package security

import (
	"net/http"
	"testing"
)

func TestClientIPTrustedProxy(t *testing.T) {
	cases := []struct {
		name, remote, forwarded, want string
		trusted []string
	}{
		{"direct", "192.168.108.134:1234", "1.2.3.4", "192.168.108.134", nil},
		{"local", "127.0.0.1:1234", "192.168.108.134", "192.168.108.134", []string{"127.0.0.1/32"}},
		{"spoofed", "127.0.0.1:1234", "1.2.3.4, 192.168.108.134", "192.168.108.134", []string{"127.0.0.1/32"}},
		{"chain", "127.0.0.1:1234", "192.168.108.134, 10.0.0.5", "192.168.108.134", []string{"127.0.0.1/32", "10.0.0.0/24"}},
		{"untrusted", "127.0.0.1:1234", "192.168.108.134", "127.0.0.1", nil},
		{"ipv6", "[::1]:1234", "192.168.108.134", "192.168.108.134", []string{"::1/128"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := NewTrustedProxy(tc.trusted)
			if err != nil { t.Fatal(err) }
			r := &http.Request{RemoteAddr: tc.remote, Header: make(http.Header)}
			r.Header.Set("X-Forwarded-For", tc.forwarded)
			got := p.ClientIP(r)
			if got == nil || got.String() != tc.want { t.Fatalf("got %v want %s", got, tc.want) }
		})
	}
}
