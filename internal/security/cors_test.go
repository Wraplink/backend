package security

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCORSAllowsDepositIdempotencyKeyPreflight(t *testing.T) {
	cors := NewCORS([]string{"http://localhost:3000"})
	handler := cors.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("preflight should not reach application handler")
	}))

	request := httptest.NewRequest(http.MethodOptions, "/api/v1/me/deposits", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Access-Control-Request-Method", "POST")
	request.Header.Set("Access-Control-Request-Headers", "authorization, content-type, idempotency-key")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("allowed origin = %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("allow credentials = %q", got)
	}
	allowedHeaders := strings.ToLower(recorder.Header().Get("Access-Control-Allow-Headers"))
	if !strings.Contains(allowedHeaders, "idempotency-key") {
		t.Fatalf("preflight does not allow deposit idempotency key: %q", allowedHeaders)
	}
}

func TestCORSRejectsUnknownOriginPreflight(t *testing.T) {
	cors := NewCORS([]string{"http://localhost:3000"})
	handler := cors.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("preflight should not reach application handler")
	}))
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/me/deposits", nil)
	request.Header.Set("Origin", "https://not-allowed.example")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("unknown origin status = %d, want 403", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unknown origin was allowed: %q", got)
	}
}
