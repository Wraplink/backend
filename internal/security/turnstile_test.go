package security

import (
    "net/http"
    "net/http/httptest"
    "os"
    "testing"
)

func TestTurnstileMissingTokenIsRejected(t *testing.T) {
    old := os.Getenv("TURNSTILE_SECRET_KEY")
    t.Setenv("TURNSTILE_SECRET_KEY", "test-secret")
    defer os.Setenv("TURNSTILE_SECRET_KEY", old)
    middleware := TurnstileMiddleware(nil)
    called := false
    handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        called = true
    }))
    req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)
    if rec.Code != http.StatusForbidden || called {
        t.Fatalf("expected blocked request; got %d, called=%v", rec.Code, called)
    }
}
