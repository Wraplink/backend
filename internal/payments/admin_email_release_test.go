package payments

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"backend/internal/auth"
	"backend/internal/config"
	"backend/internal/email"
	"backend/internal/legal"
	"backend/internal/requestcontext"
	"backend/internal/token"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAdminEmailReleasePostgresIntegration(t *testing.T) {
	dsn := os.Getenv("WRAPLINK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("WRAPLINK_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	createUser := func(status string) (uuid.UUID, string) {
		t.Helper()
		id := uuid.New()
		address := id.String() + "@example.com"
		exec("INSERT INTO users (id, email, full_name, account_status) VALUES ($1, $2, 'Email Release Test', $3)",
			id, address, status)
		return id, address
	}
	admin, _ := createUser("active")
	oldUser, address := createUser("deleted")
	var newUser uuid.UUID
	defer func() {
		_, _ = db.Exec(context.Background(), "DELETE FROM security_events WHERE user_id IN ($1, $2, $3)", admin, oldUser, newUser)
		_, _ = db.Exec(context.Background(), "DELETE FROM users WHERE id IN ($1, $2, $3)", admin, oldUser, newUser)
	}()

	exec("INSERT INTO access_admins (user_id) VALUES ($1)", admin)
	exec("INSERT INTO google_identities (subject, user_id) VALUES ($1, $2)", oldUser.String(), oldUser)
	exec("INSERT INTO security_events (user_id, event_type) VALUES ($1, 'ACCOUNT_DELETED')", oldUser)

	staleToken := oldUser.String()
	exec("INSERT INTO email_verifications (user_id, token_hash, expires_at) VALUES ($1, $2, now()+interval '1 hour')",
		oldUser, token.Hash(staleToken))

	h := NewHandler(NewService(NewRepository(db)))
	router := chi.NewRouter()
	router.Patch("/users/{userID}/status", h.AdminUpdateUserStatus)
	router.Get("/users", h.AdminUsers)
	send := func(method, path string, actor uuid.UUID, status int) string {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(`{"action":"release_email"}`))
		req = req.WithContext(requestcontext.WithUserID(ctx, actor))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("status=%d, want %d, body=%s", w.Code, status, w.Body.String())
		}
		return w.Body.String()
	}
	releasePath := "/users/" + oldUser.String() + "/status"
	send(http.MethodPatch, releasePath, oldUser, http.StatusForbidden)

	ip := net.ParseIP("192.0.2.5")
	request := auth.RegisterRequest{
		FullName: "New Email Owner", Email: address, Password: "Example123!Password",
		TermsVersion: legal.CurrentTermsVersion, PrivacyVersion: legal.CurrentPrivacyVersion,
		Locale: "en", IPAddress: ip,
	}
	registration := auth.NewRegistrationService(db, email.NewLogSender(slog.New(slog.NewTextHandler(io.Discard, nil)), "https://example.com"))
	if _, err := registration.Register(ctx, request); !errors.Is(err, auth.ErrEmailAlreadyExists) {
		t.Fatalf("expected unreleased email to block registration, got %v", err)
	}
	send(http.MethodPatch, releasePath, admin, http.StatusOK)
	send(http.MethodPatch, releasePath, admin, http.StatusOK)

	if body := send(http.MethodGet, "/users?search="+releasedEmailPrefix+oldUser.String(), admin, http.StatusOK); !strings.Contains(body, `"emailReleased":true`) {
		t.Fatalf("directory did not report release: %s", body)
	}
	var state, placeholder string
	var linked bool
	var events int
	err = db.QueryRow(ctx, `SELECT account_status, email,
		EXISTS (SELECT 1 FROM google_identities WHERE user_id=$1),
		(SELECT count(*) FROM security_events WHERE user_id=$1 AND event_type IN ('ACCOUNT_DELETED','ADMIN_DELETED_ACCOUNT_EMAIL_RELEASED'))
		FROM users WHERE id=$1`, oldUser).Scan(&state, &placeholder, &linked, &events)
	if err != nil || state != "deleted" || placeholder != releasedEmailPrefix+oldUser.String() || linked || events != 2 {
		t.Fatalf("unexpected released account: state=%q, placeholder=%q, linked=%t, events=%d, err=%v",
			state, placeholder, linked, events, err)
	}

	verify := auth.NewEmailVerificationService(db)
	if err := verify.Verify(ctx, auth.VerifyEmailRequest{Token: staleToken, IPAddress: ip}); !errors.Is(err, auth.ErrInvalidVerificationToken) {
		t.Fatalf("expected old verification token to be rejected, got %v", err)
	}

	registered, err := registration.Register(ctx, request)
	if err != nil {
		t.Fatalf("register released email: %v", err)
	}
	newUser = registered.UserID
	if newUser == oldUser {
		t.Fatal("registration reused the deleted account ID")
	}
	if err := verify.Verify(ctx, auth.VerifyEmailRequest{Token: registered.EmailVerification, IPAddress: ip}); err != nil {
		t.Fatalf("verify new registration: %v", err)
	}
	send(http.MethodPatch, "/users/"+newUser.String()+"/status", admin, http.StatusConflict)

	cfg := config.Config{Security: config.SecurityConfig{
		JWTSecret: "email-release-test-secret", AccessTokenTTL: time.Minute, RefreshTokenTTL: time.Hour,
	}}
	login := auth.NewLoginService(auth.NewLoginRepository(db), auth.NewTokenService(cfg), auth.NewSessionService(db), cfg.Security)
	result, err := login.Login(ctx, auth.LoginRequest{Email: address, Password: request.Password, IPAddress: ip})
	if err != nil || result.UserID != newUser {
		t.Fatalf("expected login to new account, got %v, err=%v", result, err)
	}
}
