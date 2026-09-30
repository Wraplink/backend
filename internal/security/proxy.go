package security

import (
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

	forwarded := r.Header.Get("X-Forwarded-For")

	if forwarded == "" {
		return ip
	}

	// X-Forwarded-For format:
	//
	// client, proxy1, proxy2
	//
	// The left-most address is the original client.
	forwardedIP := forwardedAddress(forwarded)

	if forwardedIP == nil {
		return ip
	}

	return forwardedIP
}

func (p *TrustedProxy) isTrusted(ip net.IP) bool {
	for _, network := range p.networks {
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

func forwardedAddress(
	value string,
) net.IP {
	forwarded := splitForwarded(value)

	for _, value := range forwarded {
		ip := net.ParseIP(value)
		if ip != nil {
			return ip
		}
	}

	return nil
}

func splitForwarded(value string) []string {
	var result []string

	start := 0

	for i := 0; i <= len(value); i++ {
		if i == len(value) || value[i] == ',' {
			part := value[start:i]

			for len(part) > 0 && part[0] == ' ' {
				part = part[1:]
			}

			for len(part) > 0 && part[len(part)-1] == ' ' {
				part = part[:len(part)-1]
			}

			if part != "" {
				result = append(result, part)
			}

			start = i + 1
		}
	}

	return result
}
