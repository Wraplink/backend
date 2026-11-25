package auth

import (
	"backend/internal/config"
	"backend/internal/legal"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type googleTestTransport func(*http.Request) (*http.Response, error)

func (f googleTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestGoogleRegistrationAndLogin(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable migrated PostgreSQL database")
	}
	ctx := t.Context()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	email := uuid.NewString() + "@gmail.com"
	subject := uuid.NewString()
	var originalSetting bool
	if err := db.QueryRow(ctx, "SELECT enabled FROM google_auth_settings WHERE singleton").Scan(&originalSetting); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, "UPDATE google_auth_settings SET enabled = $1 WHERE singleton", originalSetting)
	defer db.Exec(ctx, "DELETE FROM users WHERE email = $1", email)

	cfg := config.Config{Security: config.SecurityConfig{
		JWTSecret: "google-auth-integration-test-secret", AccessTokenTTL: time.Minute, RefreshTokenTTL: time.Hour,
	}}
	sessions := NewSessionService(db)
	tokens := NewTokenService(cfg)
	login := NewLoginService(NewLoginRepository(db), tokens, sessions, cfg.Security)
	h := NewGoogleAuthHandler(db, login, slog.New(slog.DiscardHandler), "https://app.example.com",
		"test-client", "test-secret", "https://app.example.com"+googleCallbackPath, true)

	var expectedChallenge string
	h.client.Transport = googleTestTransport(func(r *http.Request) (*http.Response, error) {
		var body []byte
		switch r.URL.String() {
		case "https://oauth2.googleapis.com/token":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Method != http.MethodPost || r.Form.Get("code") != "test-code" ||
				r.Form.Get("client_secret") != "test-secret" ||
				r.Form.Get("redirect_uri") != h.redirectURI ||
				googleCodeChallenge(r.Form.Get("code_verifier")) != expectedChallenge {
				t.Fatal("token exchange did not match the authorization request")
			}
			body = []byte(`{"access_token":"test-token","token_type":"Bearer"}`)
		case "https://openidconnect.googleapis.com/v1/userinfo":
			if r.Header.Get("Authorization") != "Bearer test-token" {
				t.Fatal("missing Google bearer token")
			}
			body, _ = json.Marshal(googleProfile{Subject: subject, Email: email, EmailVerified: true, Name: "Google Test"})
		default:
			t.Fatalf("unexpected outbound URL %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})

	update := func(enabled bool) {
		t.Helper()
		body, _ := json.Marshal(map[string]bool{"enabled": enabled})
		response := httptest.NewRecorder()
		h.UpdateSettings(response, httptest.NewRequest(http.MethodPut, "/settings", strings.NewReader(string(body))))
		if response.Code != http.StatusOK {
			t.Fatalf("update settings: %d %s", response.Code, response.Body)
		}
	}
	start := func(register bool, input googleStartRequest, wantStatus int) (*http.Cookie, string) {
		t.Helper()
		body, _ := json.Marshal(input)
		request := httptest.NewRequest(http.MethodPost, "/start", strings.NewReader(string(body)))
		request.Header.Set("Origin", "https://app.example.com")
		response := httptest.NewRecorder()
		if register {
			h.StartRegistration(response, request)
		} else {
			h.StartLogin(response, request)
		}
		if response.Code != wantStatus {
			t.Fatalf("start: got %d, want %d (%s)", response.Code, wantStatus, response.Body)
		}
		if wantStatus != http.StatusOK {
			return nil, ""
		}
		var result struct {
			AuthorizationURL string `json:"authorizationUrl"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		authorization, err := url.Parse(result.AuthorizationURL)
		if err != nil {
			t.Fatal(err)
		}
		query := authorization.Query()
		cookies := response.Result().Cookies()
		if authorization.Host != "accounts.google.com" || query.Get("code_challenge_method") != "S256" ||
			len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure {
			t.Fatal("missing Google authorization parameters or protected flow cookie")
		}
		expectedChallenge = query.Get("code_challenge")
		return cookies[0], query.Get("state")
	}
	callback := func(cookie *http.Cookie, state string, failed bool) *http.Cookie {
		t.Helper()
		query := url.Values{"code": {"test-code"}, "state": {state}}
		request := httptest.NewRequest(http.MethodGet, googleCallbackPath+"?"+query.Encode(), nil)
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		h.Callback(response, request)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("callback: %d %s", response.Code, response.Body)
		}
		location, err := url.Parse(response.Header().Get("Location"))
		if err != nil || location.Host != "app.example.com" || (location.Query().Get("googleError") == "1") != failed {
			t.Fatalf("unexpected callback redirect %q", response.Header().Get("Location"))
		}
		for _, cookie := range response.Result().Cookies() {
			if cookie.Name == refreshCookieName {
				if failed {
					t.Fatal("failed callback created a refresh cookie")
				}
				if location.Path != "/en/auth/login" || location.Query().Get("returnUrl") != "/checkout" {
					t.Fatalf("callback lost locale or destination: %s", location)
				}
				return cookie
			}
		}
		if !failed {
			t.Fatal("successful callback did not create a session")
		}
		return nil
	}

	input := googleStartRequest{Locale: "en", ReturnURL: "/checkout"}
	update(false)
	start(false, input, http.StatusServiceUnavailable)
	update(true)
	start(true, input, http.StatusBadRequest)
	cookie, state := start(false, input, http.StatusOK)
	callback(cookie, state, true) // Login cannot create an account.

	input.TermsVersion = legal.CurrentTermsVersion
	input.PrivacyVersion = legal.CurrentPrivacyVersion
	input.MarketingConsent = true
	cookie, state = start(true, input, http.StatusOK)
	callback(cookie, state+"tampered", true)
	refresh := callback(cookie, state, false)
	if refresh.MaxAge != 0 {
		t.Fatal("registration unexpectedly persisted the refresh cookie")
	}

	var userID uuid.UUID
	var marketing bool
	err = db.QueryRow(ctx, `
		SELECT u.id, l.marketing_consent
		FROM users u JOIN legal_acceptances l ON l.user_id = u.id
		WHERE u.email = $1 AND u.email_verified AND u.account_status = 'active'
		AND l.terms_version = $2 AND l.privacy_version = $3
	`, email, input.TermsVersion, input.PrivacyVersion).Scan(&userID, &marketing)
	if err != nil || !marketing {
		t.Fatalf("registration did not record the active account and consent: %v", err)
	}
	if created, err := NewPasswordResetRepository(db).Create(ctx, userID, uuid.NewString()); err != nil || created {
		t.Fatalf("Google-only account accepted a password reset: created=%v err=%v", created, err)
	}

	input.RememberDevice = true
	cookie, state = start(false, input, http.StatusOK)
	refresh = callback(cookie, state, false)
	if refresh.MaxAge <= 0 {
		t.Fatal("Google login did not honor remembered-device preference")
	}
	restored, err := NewRefreshService(sessions, tokens, cfg.Security).Refresh(ctx, refresh.Value, nil, "")
	if err != nil || restored.UserID != userID.String() {
		t.Fatalf("Google session could not be restored: %v", err)
	}

	cookie, state = start(false, input, http.StatusOK)
	update(false)
	callback(cookie, state, true)

	update(true)
	subject = uuid.NewString()
	cookie, state = start(true, input, http.StatusOK)
	callback(cookie, state, true) // A different Google subject cannot take over the email.
}

func TestSafeGoogleReturnURL(t *testing.T) {
	for _, target := range []string{"https://example.com", "//example.com", "/\\example.com", "/%5cexample.com", "/%2fexample.com", "/a%0a"} {
		if got := safeGoogleReturnURL(target); got != "" {
			t.Errorf("accepted unsafe destination %q", target)
		}
	}
	if got := safeGoogleReturnURL("/checkout?plan=one"); got != "/checkout?plan=one" {
		t.Errorf("lost internal destination: %q", got)
	}
}
