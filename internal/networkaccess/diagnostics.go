package networkaccess

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"backend/internal/requestcontext"

	"github.com/go-chi/chi/v5"
)

const diagnosticTimeout = 3 * time.Second

// DiagnosticResponse always describes a check made from the backend host,
// not an ICMP ping or traceroute from the customer's device.
type DiagnosticResponse struct {
	Success bool    `json:"success"`
	Value   string  `json:"value"`
	Latency float64 `json:"latency,omitempty"`
	Scope   string  `json:"scope"`
}

// Diagnostic performs bounded checks against server-selected DNS endpoints.
// The request never accepts a host, port or arbitrary target from the client.
func (h *Handler) Diagnostic(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "type")
	if kind != "ping" && kind != "dns" && kind != "route" {
		writeJSONError(w, http.StatusBadRequest, "unsupported diagnostic")
		return
	}
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	access, err := h.service.Access(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrNoActiveSubscription) {
			writeJSONError(w, http.StatusNotFound, "no active subscription")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "could not load network access")
		return
	}
	if access.Status != AccessStatusActive {
		writeJSON(w, http.StatusOK, DiagnosticResponse{Value: "Network subscription is not active", Scope: "backend"})
		return
	}

	servers := make([]string, 0, 2)
	if access.Connection != nil {
		for _, address := range access.Connection.DNSServers {
			if ip := net.ParseIP(strings.TrimSpace(address)); ip != nil {
				servers = append(servers, ip.String())
			}
		}
	}
	if len(servers) == 0 {
		writeJSON(w, http.StatusOK, DiagnosticResponse{Value: "No WrapLink DNS address is assigned", Scope: "backend"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), diagnosticTimeout)
	defer cancel()

	var result DiagnosticResponse
	switch kind {
	case "ping":
		result = checkDNSPort(ctx, servers)
	case "dns":
		result = checkDNSResolution(ctx, servers)
	case "route":
		result = h.checkRouteReadiness(servers)
	}
	writeJSON(w, http.StatusOK, result)
}

// This is TCP/53 connection latency, not ICMP round-trip time.
func checkDNSPort(ctx context.Context, servers []string) DiagnosticResponse {
	result := DiagnosticResponse{Value: "DNS TCP/53 is unreachable from the backend", Scope: "backend"}
	dialer := &net.Dialer{Timeout: diagnosticTimeout}
	for _, address := range servers {
		start := time.Now()
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(address, "53"))
		if err != nil {
			continue
		}
		_ = conn.Close()
		result.Success = true
		result.Latency = float64(time.Since(start).Microseconds()) / 1000
		result.Value = fmt.Sprintf("TCP/53 reachable at %s (backend to DNS)", address)
		return result
	}
	return result
}

// Query a fixed public domain using exactly the assigned DNS server.
// No user-selected hostname can trigger arbitrary outbound requests.
func checkDNSResolution(ctx context.Context, servers []string) DiagnosticResponse {
	result := DiagnosticResponse{Value: "DNS lookup failed through assigned WrapLink resolvers", Scope: "backend"}
	for _, address := range servers {
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				dialer := &net.Dialer{Timeout: diagnosticTimeout}
				return dialer.DialContext(ctx, network, net.JoinHostPort(address, "53"))
			},
		}
		start := time.Now()
		ips, err := resolver.LookupIPAddr(ctx, "www.playstation.com.")
		if err != nil || len(ips) == 0 {
			continue
		}
		result.Success = true
		result.Latency = float64(time.Since(start).Microseconds()) / 1000
		result.Value = fmt.Sprintf("www.playstation.com resolved via %s (backend to DNS)", address)
		return result
	}
	return result
}

// This verifies that the assigned resolver is advertised by a healthy
// dns-control node. It does not claim to trace the user's actual route.
func (h *Handler) checkRouteReadiness(servers []string) DiagnosticResponse {
	result := DiagnosticResponse{Value: "No healthy registered DNS control route for assigned DNS", Scope: "control-plane"}
	if h.service.dnsControls == nil {
		return result
	}
	nodes := h.service.dnsControls.List(h.service.dnsControlOfflineAfter)
	for _, node := range nodes {
		for _, assigned := range servers {
			for _, advertised := range node.Addresses {
				if assigned == advertised {
					result.Success = true
					result.Value = fmt.Sprintf("Assigned DNS %s advertised by healthy node %s; client route not measured", assigned, node.ID)
					return result
				}
			}
		}
	}
	return result
}
