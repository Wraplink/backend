package publicip

import (
 "errors"
 "net"
 "net/http"
 "net/netip"
 "strings"
)

var ErrUntrustedForwardedAddress = errors.New("invalid or untrusted forwarded client address")

// TrustedClientIPv4 resolves a public client IP using a configured trusted proxy
// network. The proxy MUST overwrite X-Real-IP with its validated client address.
// Forwarded headers from untrusted peers are never used. If the peer is itself
// public and untrusted, its direct IP is returned.
func TrustedClientIPv4(r *http.Request, trustedProxies []netip.Prefix) (string, error) {
 host, _, err := net.SplitHostPort(r.RemoteAddr)
 if err != nil { return "", ErrUntrustedForwardedAddress }
 peer, err := netip.ParseAddr(host)
 if err != nil { return "", ErrUntrustedForwardedAddress }
 peer = peer.Unmap()
 trusted := false
 for _, prefix := range trustedProxies {
  if prefix.Contains(peer) { trusted = true; break }
 }
 if !trusted {
  ip, err := Parse(peer.String())
  if err != nil { return "", err }
  return ip.String(), nil
 }
 // A trusted reverse proxy must set exactly one canonical X-Real-IP value.
 // Reject ambiguous/multiple values rather than falling back to an internal
 // proxy address or interpreting client-controlled X-Forwarded-For.
 values := r.Header.Values("X-Real-Ip")
 if len(values) != 1 || strings.Contains(values[0], ",") {
  return "", ErrUntrustedForwardedAddress
 }
 ip, err := Parse(values[0])
 if err != nil { return "", ErrUntrustedForwardedAddress }
 return ip.String(), nil
}
