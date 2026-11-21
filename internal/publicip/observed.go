package publicip

import (
 "net"
 "net/http"
)

// ObservedIPv4 returns the direct TCP peer address. Reverse proxy forwarding
// headers are deliberately ignored; a trusted proxy integration must explicitly
// validate and normalize forwarded addresses before passing them here.
func ObservedIPv4(r *http.Request) (string, error) {
 host, _, err := net.SplitHostPort(r.RemoteAddr)
 if err != nil { return "", ErrInvalidPublicIPv4 }
 ip, err := Parse(host)
 if err != nil { return "", err }
 return ip.String(), nil
}
