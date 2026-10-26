package routeinventory

import (
 "context"
 "errors"
 "fmt"
 "net"
 "time"
)

// resolveOriginalIPv4 bypasses the host's possibly hijacked resolver.
// Each upstream has its own timeout so a blocked primary does not
// prevent fallback to another independent DNS server.
func resolveOriginalIPv4(ctx context.Context, domain string) (string, error) {
 var failures []error
 for _, upstream := range []string{"1.1.1.1:53", "8.8.8.8:53"} {
  lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
  resolver := &net.Resolver{PreferGo: true, Dial: func(c context.Context, network, address string) (net.Conn, error) {
   return (&net.Dialer{Timeout: 2*time.Second}).DialContext(c, "udp", upstream)
  }}
  addresses, err := resolver.LookupIP(lookupCtx, "ip4", domain)
  cancel()
  if err == nil {
   for _, address := range addresses {
    if ipv4 := address.To4(); ipv4 != nil {
     return ipv4.String(), nil
    }
   }
   err = errors.New("no IPv4 address returned")
  }
  failures = append(failures, fmt.Errorf("%s: %w", upstream, err))
 }
 return "", fmt.Errorf("resolve %q: %w", domain, errors.Join(failures...))
}
