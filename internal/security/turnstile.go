package security

import (
    "encoding/json"
    "net/http"
    "net/url"
    "os"
    "strings"
    "time"
)

// TurnstileMiddleware verifies single-use challenge tokens before public mutations.
// Configure TURNSTILE_SECRET_KEY and NEXT_PUBLIC_TURNSTILE_SITE_KEY together.
// A missing secret disables the feature for local development.
func TurnstileMiddleware(trustedProxy *TrustedProxy) func(http.Handler) http.Handler {
    secret := strings.TrimSpace(os.Getenv("TURNSTILE_SECRET_KEY"))
    client := &http.Client{Timeout: 5 * time.Second}
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if secret == "" {
                next.ServeHTTP(w, r)
                return
            }
            token := strings.TrimSpace(r.Header.Get("X-Turnstile-Token"))
            if token == "" || len(token) > 2048 {
                http.Error(w, `{"error":"captcha required"}`, http.StatusForbidden)
                return
            }
            values := url.Values{"secret": {secret}, "response": {token}}
            if ip := trustedProxy.ClientIP(r); ip != nil {
                values.Set("remoteip", ip.String())
            }
            req, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
                "https://challenges.cloudflare.com/turnstile/v0/siteverify",
                strings.NewReader(values.Encode()))
            if err != nil {
                http.Error(w, `{"error":"captcha unavailable"}`, http.StatusServiceUnavailable)
                return
            }
            req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
            resp, err := client.Do(req)
            if err != nil {
                http.Error(w, `{"error":"captcha unavailable"}`, http.StatusServiceUnavailable)
                return
            }
            defer resp.Body.Close()
            var result struct { Success bool `json:"success"` }
            if resp.StatusCode != http.StatusOK ||
                json.NewDecoder(http.MaxBytesReader(w, resp.Body, 4096)).Decode(&result) != nil ||
                !result.Success {
                http.Error(w, `{"error":"captcha verification failed"}`, http.StatusForbidden)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
