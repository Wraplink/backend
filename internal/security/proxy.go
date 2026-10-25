package security

import (
	"strings"
	"net"
	"net/http"
)

type TrustedProxy struct {
	networks []*net.IPNet
}

func NewTrustedProxy(
	cidrs []string,
) (*TrustedProxy, error) {
	networks := make([]*net.IPNet, 0, len(cidrs))

	for _, value := range cidrs {
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			return nil, err
		}

		networks = append(networks, network)
	}

	return &TrustedProxy{
		networks: networks,
	}, nil
}

func (p *TrustedProxy) ClientIP(
	r *http.Request,
) net.IP {
	remoteIP := RemoteIP(r)

	if remoteIP == "" {
		return nil
	}

	ip := net.ParseIP(remoteIP)
	if ip == nil {
		return nil
	}

	// No trusted proxy configuration:
	// always trust the direct TCP peer.
	if !p.isTrusted(ip) {
		return ip
	}

	// Evaluate the complete chain from the nearest proxy backwards.
	// A client-controlled leftmost entry must never override an untrusted hop.
	values := r.Header.Values("X-Forwarded-For")
	if len(values) == 0 {
		return ip
	}
	chain := strings.Join(values, ",")
	parts := strings.Split(chain, ",")
	current := ip
	for i := len(parts) - 1; i >= 0; i-- {
		if !p.isTrusted(current) {
			return current
		}
		next := net.ParseIP(strings.TrimSpace(parts[i]))
		if next == nil {
			// Fail closed on malformed forwarding information.
			return nil
		}
		current = next
	}
	return current
}

func (p *TrustedProxy) isTrusted(ip net.IP) bool {
	for _, network := range p.networks {
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

