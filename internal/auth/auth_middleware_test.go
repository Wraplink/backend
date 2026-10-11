package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"backend/internal/requestcontext"

	"github.com/google/uuid"
)

type mockActiveSessionChecker struct {
	active bool
	err    error
	calls  int
	userID uuid.UUID
	sessID uuid.UUID
}

func (m *mockActiveSessionChecker) IsActive(
	ctx context.Context,
	userID uuid.UUID,
	sessionID uuid.UUID,
) (bool, error) {
	m.calls++
	m.userID = userID
	m.sessID = sessionID
	return m.active, m.err
}

func newTestTokenService(ttl time.Duration) *TokenService {
	return &TokenService{
		secret:          []byte("test-secret"),
		ttl:             ttl,
		refreshTokenTTL: 24 * time.Hour,
	}
}

func TestBearerToken(t *testing.T) {
	tests := []struct {
		name          string
		authorization string
		wantToken     string
		wantOK        bool
	}{
		{"valid bearer token", "Bearer abc.def.ghi", "abc.def.ghi", true},
		{"lowercase bearer", "bearer abc.def.ghi", "abc.def.ghi", true},
		{"mixed case bearer", "BeArEr abc.def.ghi", "abc.def.ghi", true},
		{"extra spaces", "  Bearer   abc.def.ghi  ", "abc.def.ghi", true},
		{"missing authorization", "", "", false},
		{"basic authentication", "Basic abc.def.ghi", "", false},
		{"missing token", "Bearer", "", false},
		{"too many fields", "Bearer abc.def.ghi extra", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, ok := bearerToken(tt.authorization)

			if token != tt.wantToken {
				t.Fatalf("bearerToken() token = %q, want %q", token, tt.wantToken)
			}
			if ok != tt.wantOK {
				t.Fatalf("bearerToken() ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

func TestAuthMiddlewareMissingAuthorization(t *testing.T) {
	checker := &mockActiveSessionChecker{active: true}
	middleware := NewAuthMiddleware(newTestTokenService(time.Hour), checker)

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	rec := httptest.NewRecorder()

	middleware.Middleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if nextCalled {
		t.Fatal("next handler was called")
	}
	if checker.calls != 0 {
		t.Fatalf("session checker called %d times, want 0", checker.calls)
	}
}

func TestAuthMiddlewareInvalidToken(t *testing.T) {
	checker := &mockActiveSessionChecker{active: true}
	middleware := NewAuthMiddleware(newTestTokenService(time.Hour), checker)

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()

	middleware.Middleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if nextCalled {
		t.Fatal("next handler was called")
	}
	if checker.calls != 0 {
		t.Fatalf("session checker called %d times, want 0", checker.calls)
	}
}

func TestAuthMiddlewareValidToken(t *testing.T) {
	tokenService := newTestTokenService(time.Hour)
	userID := uuid.New()
	sessionID := uuid.New()
	checker := &mockActiveSessionChecker{active: true}

	token, err := tokenService.CreateAccessToken(userID.String(), sessionID.String())
	if err != nil {
		t.Fatalf("CreateAccessToken() error = %v", err)
	}

	middleware := NewAuthMiddleware(tokenService, checker)
	nextCalled := false

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true

		gotUserID, ok := requestcontext.UserID(r.Context())
		if !ok {
			t.Fatal("user ID not found in request context")
		}
		if gotUserID != userID {
			t.Fatalf("user ID = %s, want %s", gotUserID, userID)
		}

		gotSessionID, ok := requestcontext.SessionID(r.Context())
		if !ok {
			t.Fatal("session ID not found in request context")
		}
		if gotSessionID != sessionID {
			t.Fatalf("session ID = %s, want %s", gotSessionID, sessionID)
		}

		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	middleware.Middleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !nextCalled {
		t.Fatal("next handler was not called")
	}
	if checker.calls != 1 {
		t.Fatalf("session checker called %d times, want 1", checker.calls)
	}
	if checker.userID != userID {
		t.Fatalf("checker user ID = %s, want %s", checker.userID, userID)
	}
	if checker.sessID != sessionID {
		t.Fatalf("checker session ID = %s, want %s", checker.sessID, sessionID)
	}
}

func TestAuthMiddlewareExpiredToken(t *testing.T) {
	tokenService := newTestTokenService(-time.Hour)
	checker := &mockActiveSessionChecker{active: true}
	userID := uuid.New()
	sessionID := uuid.New()

	token, err := tokenService.CreateAccessToken(userID.String(), sessionID.String())
	if err != nil {
		t.Fatalf("CreateAccessToken() error = %v", err)
	}

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	NewAuthMiddleware(tokenService, checker).Middleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if nextCalled {
		t.Fatal("next handler was called")
	}
	if checker.calls != 0 {
		t.Fatalf("session checker called %d times, want 0", checker.calls)
	}
}

func TestAuthMiddlewareInvalidUserID(t *testing.T) {
	tokenService := newTestTokenService(time.Hour)
	sessionID := uuid.New()
	checker := &mockActiveSessionChecker{active: true}

	token, err := tokenService.CreateAccessToken("not-a-uuid", sessionID.String())
	if err != nil {
		t.Fatalf("CreateAccessToken() error = %v", err)
	}

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	NewAuthMiddleware(tokenService, checker).Middleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if nextCalled {
		t.Fatal("next handler was called")
	}
	if checker.calls != 0 {
		t.Fatalf("session checker called %d times, want 0", checker.calls)
	}
}

func TestAuthMiddlewareInactiveSession(t *testing.T) {
	tokenService := newTestTokenService(time.Hour)
	userID := uuid.New()
	sessionID := uuid.New()
	checker := &mockActiveSessionChecker{active: false}

	token, err := tokenService.CreateAccessToken(userID.String(), sessionID.String())
	if err != nil {
		t.Fatalf("CreateAccessToken() error = %v", err)
	}

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	NewAuthMiddleware(tokenService, checker).Middleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if nextCalled {
		t.Fatal("next handler was called for an inactive session")
	}
	if checker.calls != 1 {
		t.Fatalf("session checker called %d times, want 1", checker.calls)
	}
}

func TestAuthMiddlewareSessionCheckerError(t *testing.T) {
	tokenService := newTestTokenService(time.Hour)
	userID := uuid.New()
	sessionID := uuid.New()
	checker := &mockActiveSessionChecker{
		err: errors.New("database unavailable"),
	}

	token, err := tokenService.CreateAccessToken(userID.String(), sessionID.String())
	if err != nil {
		t.Fatalf("CreateAccessToken() error = %v", err)
	}

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	NewAuthMiddleware(tokenService, checker).Middleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if nextCalled {
		t.Fatal("next handler was called when session check failed")
	}
}

func TestAuthMiddlewareInvalidSessionID(t *testing.T) {
	tokenService := newTestTokenService(time.Hour)
	userID := uuid.New()
	checker := &mockActiveSessionChecker{active: true}

	token, err := tokenService.CreateAccessToken(userID.String(), "not-a-uuid")
	if err != nil {
		t.Fatalf("CreateAccessToken() error = %v", err)
	}

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	NewAuthMiddleware(tokenService, checker).Middleware(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if nextCalled {
		t.Fatal("next handler was called")
	}
	if checker.calls != 0 {
		t.Fatalf("session checker called %d times, want 0", checker.calls)
	}
}

func TestRequestContextUserID(t *testing.T) {
	userID := uuid.New()
	ctx := requestcontext.WithUserID(context.Background(), userID)

	gotUserID, ok := requestcontext.UserID(ctx)
	if !ok {
		t.Fatal("user ID not found in context")
	}
	if gotUserID != userID {
		t.Fatalf("user ID = %s, want %s", gotUserID, userID)
	}
}

func TestRequestContextUserIDMissing(t *testing.T) {
	userID, ok := requestcontext.UserID(context.Background())

	if ok {
		t.Fatal("expected user ID to be missing")
	}
	if userID != uuid.Nil {
		t.Fatalf("user ID = %s, want nil UUID", userID)
	}
}

func TestRequestContextSessionID(t *testing.T) {
	sessionID := uuid.New()
	ctx := requestcontext.WithSessionID(context.Background(), sessionID)

	gotSessionID, ok := requestcontext.SessionID(ctx)
	if !ok {
		t.Fatal("session ID not found in context")
	}
	if gotSessionID != sessionID {
		t.Fatalf("session ID = %s, want %s", gotSessionID, sessionID)
	}
}

func TestRequestContextSessionIDMissing(t *testing.T) {
	sessionID, ok := requestcontext.SessionID(context.Background())

	if ok {
		t.Fatal("expected session ID to be missing")
	}
	if sessionID != uuid.Nil {
		t.Fatalf("session ID = %s, want nil UUID", sessionID)
	}
}
