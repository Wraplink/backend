package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"backend/internal/password"
	"backend/internal/requestcontext"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAccountDeletePostgresIntegration(t *testing.T) {
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

	userID, sessionID, planID, subscriptionID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO plans (id, code, name, price_minor, currency, duration_days, data_limit_bytes)
		VALUES ($1, $1::uuid::text, 'Deletion Test Plan', 0, 'USD', 1, 1000)`, planID)
	defer func() { _, _ = db.Exec(context.Background(), "DELETE FROM plans WHERE id = $1", planID) }()
	exec("INSERT INTO users (id, email, full_name, email_verified, account_status) VALUES ($1, $2, 'Deletion Test', true, 'active')",
		userID, userID.String()+"@example.invalid")
	defer func() { _, _ = db.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID) }()

	const currentPassword = "DeletionTestPassword123!"
	hash, err := password.Hash(currentPassword)
	if err != nil {
		t.Fatal(err)
	}
	exec("INSERT INTO user_credentials (user_id, password_hash) VALUES ($1, $2)", userID, hash)
	exec(`INSERT INTO user_sessions (id, user_id, refresh_token_hash, family_id, expires_at)
		VALUES ($1, $2, $1::uuid::text, $1, now() + interval '1 hour')`, sessionID, userID)

	exec(`INSERT INTO user_subscriptions (id, user_id, plan_id, plan_code, plan_name, price_minor, currency,
		duration_days, data_limit_bytes, status, starts_at, expires_at)
		VALUES ($1, $2, $3, $3::uuid::text, 'Deletion Test Plan', 0, 'USD', 1, 1000, 'active', now(), now() + interval '1 day')`,
		subscriptionID, userID, planID)
	exec(`INSERT INTO network_entitlements (user_id, subscription_id, client_ip)
		VALUES ($1, $2, '192.0.2.53')`, userID, subscriptionID)

	handler := NewAccountDeleteHandler(db, true)
	send := func(accountID uuid.UUID, body string, authenticated bool, expectedStatus int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/me/", strings.NewReader(body))
		if authenticated {
			r = r.WithContext(requestcontext.WithUserID(ctx, accountID))
		}
		w := httptest.NewRecorder()
		handler.Delete(w, r)
		if w.Code != expectedStatus {
			t.Fatalf("status = %d, want %d (%s)", w.Code, expectedStatus, w.Body.String())
		}
		return w
	}
	const validBody = `{"currentPassword":"` + currentPassword + `"}`
	send(userID, validBody, false, http.StatusUnauthorized)
	send(userID, `{"currentPassword":"wrong"}`, true, http.StatusForbidden)
	send(userID, `{"confirmationEmail":"`+userID.String()+`@example.invalid"}`, true, http.StatusForbidden)
	exec("INSERT INTO access_admins (user_id) VALUES ($1)", userID)
	send(userID, validBody, true, http.StatusConflict)
	exec("DELETE FROM access_admins WHERE user_id = $1", userID)

	w := send(userID, validBody, true, http.StatusNoContent)
	cookies := w.Result().Cookies()
	if len(cookies) != 2 || cookies[1].Name != refreshCookieName || cookies[1].MaxAge >= 0 {
		t.Fatalf("expected cleared refresh and remember-device cookies, got %v", cookies)
	}
	var status string
	var credentialsExist, sessionRevoked, entitlementRevoked, audited bool
	err = db.QueryRow(ctx, `SELECT account_status,
		EXISTS (SELECT 1 FROM user_credentials WHERE user_id = $1),
		(SELECT revoked_at IS NOT NULL FROM user_sessions WHERE id = $2),
		(SELECT status = 'revoked' FROM network_entitlements WHERE subscription_id = $3),
		EXISTS (SELECT 1 FROM security_events WHERE user_id = $1 AND event_type = 'ACCOUNT_DELETED')
		FROM users WHERE id = $1`, userID, sessionID, subscriptionID).Scan(&status, &credentialsExist, &sessionRevoked, &entitlementRevoked, &audited)
	if err != nil {
		t.Fatal(err)
	}
	if status != "deleted" || credentialsExist || !sessionRevoked || !entitlementRevoked || !audited {
		t.Fatalf("unexpected deletion state: status=%s, credentials=%t, session revoked=%t, entitlement revoked=%t, audited=%t",
			status, credentialsExist, sessionRevoked, entitlementRevoked, audited)
	}
	googleUserID := uuid.New()
	googleEmail := googleUserID.String() + "@example.invalid"
	exec("INSERT INTO users (id, email, full_name, email_verified, account_status) VALUES ($1, $2, 'Google Deletion Test', true, 'active')",
		googleUserID, googleEmail)
	defer func() { _, _ = db.Exec(context.Background(), "DELETE FROM users WHERE id = $1", googleUserID) }()
	exec("INSERT INTO google_identities (subject, user_id) VALUES ($1, $2)", googleUserID.String(), googleUserID)

	send(googleUserID, `{"confirmationEmail":"wrong@example.invalid"}`, true, http.StatusForbidden)
	send(googleUserID, `{"confirmationEmail":"`+googleEmail+`"}`, true, http.StatusNoContent)
	if err := db.QueryRow(ctx, "SELECT account_status FROM users WHERE id = $1", googleUserID).Scan(&status); err != nil || status != "deleted" {
		t.Fatalf("Google account deletion: status=%q, err=%v", status, err)
	}

}
