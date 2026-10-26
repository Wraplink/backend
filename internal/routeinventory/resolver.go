package routeinventory

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net"
 "net/http"
 "os"
 "strings"
 "time"
)

// resolveOriginalIPv4 asks DNS Control to resolve the original A record.
// Backend deployments behind a firewall never contact public DNS directly.
// DNS Control's origin endpoint bypasses WrapLink's managed DNS policies.
func resolveOriginalIPv4(ctx context.Context, domain string) (string, error) {
 baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("WRAPLINK_DNS_CONTROL_URL")), "/")
 token := strings.TrimSpace(os.Getenv("WRAPLINK_DNS_CONTROL_TOKEN"))
 if baseURL == "" || token == "" {
  return "", errors.New("WRAPLINK_DNS_CONTROL_URL and WRAPLINK_DNS_CONTROL_TOKEN are required for automatic route DNS")
 }
 payload, _ := json.Marshal(map[string]string{"domain":domain})
 req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/dns/origin-ipv4", bytes.NewReader(payload))
 if err != nil { return "", err }
 req.Header.Set("Content-Type", "application/json")
 req.Header.Set("Authorization", "Bearer "+token)
 client := &http.Client{Timeout:5*time.Second}
 resp, err := client.Do(req)
 if err != nil { return "", fmt.Errorf("DNS Control origin resolution: %w",err) }
 defer resp.Body.Close()
 if resp.StatusCode != http.StatusOK {
  body,_:=io.ReadAll(io.LimitReader(resp.Body,512))
  return "",fmt.Errorf("DNS Control origin resolution returned HTTP %d: %s",resp.StatusCode,strings.TrimSpace(string(body)))
 }
 var result struct { DestinationIP string `json:"destination_ip"` }
 if err:=json.NewDecoder(io.LimitReader(resp.Body,4096)).Decode(&result);err!=nil{return "",err}
 ip:=net.ParseIP(result.DestinationIP)
 if ip==nil || ip.To4()==nil {return "",errors.New("DNS Control returned invalid destination IPv4")}
 return ip.To4().String(),nil
}
