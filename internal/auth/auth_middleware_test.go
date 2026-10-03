package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"backend/internal/requestcontext"

	"github.com/google/uuid"
)

func newTestTokenService(ttl time.Duration) *TokenService {
	return &TokenService{
		secret: []byte("test-secret"),
		ttl:    ttl,
	}
}

func TestBearerToken(t *testing.T) {
	tests := []struct {
		name          string
		authorization string
		wantToken     string
		wantOK        bool
	}{
		{
			name:          "valid bearer token",
			authorization: "Bearer abc.def.ghi",
			wantToken:     "abc.def.ghi",
			wantOK:        true,
		},
		{
			name:          "lowercase bearer",
			authorization: "bearer abc.def.ghi",
			wantToken:     "abc.def.ghi",
			wantOK:        true,
		},
		{
			name:          "mixed case bearer",
			authorization: "BeArEr abc.def.ghi",
			wantToken:     "abc.def.ghi",
			wantOK:        true,
		},
		{
			name:          "extra spaces",
			authorization: "  Bearer   abc.def.ghi  ",
			wantToken:     "abc.def.ghi",
			wantOK:        true,
		},
		{
			name:          "missing authorization",
			authorization: "",
			wantToken:     "",
			wantOK:        false,
		},
		{
			name:          "basic authentication",
			authorization: "Basic abc.def.ghi",
			wantToken:     "",
			wantOK:        false,
		},
		{
			name:          "missing token",
			authorization: "Bearer",
			wantToken:     "",
			wantOK:        false,
		},
		{
			name:          "too many fields",
			authorization: "Bearer abc.def.ghi extra",
			wantToken:     "",
			wantOK:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, ok := bearerToken(tt.authorization)

			if token != tt.wantToken {
				t.Fatalf(
					"bearerToken() token = %q, want %q",
					token,
					tt.wantToken,
				)
			}

			if ok != tt.wantOK {
				t.Fatalf(
					"bearerToken() ok = %v, want %v",
					ok,
					tt.wantOK,
				)
			}
		})
	}
}

func TestAuthMiddlewareMissingAuthorization(t *testing.T) {
	tokenService := newTestTokenService(time.Hour)
	middleware := NewAuthMiddleware(tokenService)

	nextCalled := false

	next := http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.Middleware(next)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/me",
		nil,
	)

	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf(
			"status = %d, want %d",
			rec.Code,
			http.StatusUnauthorized,
		)
	}

	if nextCalled {
		t.Fatal("next handler was called")
	}
}

func TestAuthMiddlewareInvalidToken(t *testing.T) {
	tokenService := newTestTokenService(time.Hour)
	middleware := NewAuthMiddleware(tokenService)

	nextCalled := false

	next := http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.Middleware(next)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/me",
		nil,
	)

	req.Header.Set(
		"Authorization",
		"Bearer invalid-token",
	)

	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf(
			"status = %d, want %d",
			rec.Code,
			http.StatusUnauthorized,
		)
	}

	if nextCalled {
		t.Fatal("next handler was called")
	}
}

func TestAuthMiddlewareValidToken(t *testing.T) {
	tokenService := newTestTokenService(time.Hour)

	userID := uuid.New()

	token, err := tokenService.CreateAccessToken(
		userID.String(),
	)
	if err != nil {
		t.Fatalf(
			"CreateAccessToken() error = %v",
			err,
		)
	}

	middleware := NewAuthMiddleware(tokenService)

	nextCalled := false

	next := http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		nextCalled = true

		gotUserID, ok := requestcontext.UserID(
			r.Context(),
		)

		if !ok {
			t.Fatal("user ID not found in request context")
		}

		if gotUserID != userID {
			t.Fatalf(
				"user ID = %s, want %s",
				gotUserID,
				userID,
			)
		}

		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.Middleware(next)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/me",
		nil,
	)

	req.Header.Set(
		"Authorization",
		"Bearer "+token,
	)

	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"status = %d, want %d",
			rec.Code,
			http.StatusOK,
		)
	}

	if !nextCalled {
		t.Fatal("next handler was not called")
	}
}

func TestAuthMiddlewareExpiredToken(t *testing.T) {
	tokenService := newTestTokenService(-time.Hour)

	userID := uuid.New()

	token, err := tokenService.CreateAccessToken(
		userID.String(),
	)
	if err != nil {
		t.Fatalf(
			"CreateAccessToken() error = %v",
			err,
		)
	}

	middleware := NewAuthMiddleware(tokenService)

	nextCalled := false

	next := http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.Middleware(next)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/me",
		nil,
	)

	req.Header.Set(
		"Authorization",
		"Bearer "+token,
	)

	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf(
			"status = %d, want %d",
			rec.Code,
			http.StatusUnauthorized,
		)
	}

	if nextCalled {
		t.Fatal("next handler was called")
	}
}

func TestAuthMiddlewareInvalidUserID(t *testing.T) {
	tokenService := newTestTokenService(time.Hour)

	token, err := tokenService.CreateAccessToken(
		"not-a-uuid",
	)
	if err != nil {
		t.Fatalf(
			"CreateAccessToken() error = %v",
			err,
		)
	}

	middleware := NewAuthMiddleware(tokenService)

	nextCalled := false

	next := http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.Middleware(next)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/me",
		nil,
	)

	req.Header.Set(
		"Authorization",
		"Bearer "+token,
	)

	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf(
			"status = %d, want %d",
			rec.Code,
			http.StatusUnauthorized,
		)
	}

	if nextCalled {
		t.Fatal("next handler was called")
	}
}

func TestRequestContextUserID(t *testing.T) {
	userID := uuid.New()

	ctx := requestcontext.WithUserID(
		context.Background(),
		userID,
	)

	gotUserID, ok := requestcontext.UserID(ctx)

	if !ok {
		t.Fatal("user ID not found in context")
	}

	if gotUserID != userID {
		t.Fatalf(
			"user ID = %s, want %s",
			gotUserID,
			userID,
		)
	}
}

func TestRequestContextUserIDMissing(t *testing.T) {
	userID, ok := requestcontext.UserID(
		context.Background(),
	)

	if ok {
		t.Fatal("expected user ID to be missing")
	}

	if userID != uuid.Nil {
		t.Fatalf(
			"user ID = %s, want nil UUID",
			userID,
		)
	}
}
