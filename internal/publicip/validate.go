// Package publicip validates IPv4 addresses used for WrapLink client entitlements.
package publicip

import (
 "errors"
 "net/netip"
 "strings"
)

var ErrInvalidPublicIPv4 = errors.New("a globally routable public IPv4 address is required")

// Parse validates an IPv4 address for use as a user-controlled network entitlement.
// It rejects noncanonical input, IPv4-mapped IPv6, and special-use ranges.
func Parse(value string) (netip.Addr, error) {
 if value != strings.TrimSpace(value) {
  return netip.Addr{}, ErrInvalidPublicIPv4
 }
 ip, err := netip.ParseAddr(value)
 if err != nil || !ip.Is4() || !ip.IsGlobalUnicast() {
  return netip.Addr{}, ErrInvalidPublicIPv4
 }
 for _, prefix := range blockedPrefixes {
  if prefix.Contains(ip) {
   return netip.Addr{}, ErrInvalidPublicIPv4
  }
 }
 return ip, nil
}

var blockedPrefixes = func() []netip.Prefix {
 values := []string{
  "0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8",
  "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24",
  "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16",
  "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24",
  "224.0.0.0/4", "240.0.0.0/4",
 }
 out := make([]netip.Prefix, 0, len(values))
 for _, value := range values {
  out = append(out, netip.MustParsePrefix(value))
 }
 return out
}()
