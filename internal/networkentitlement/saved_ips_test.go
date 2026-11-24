package networkentitlement

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"backend/internal/requestcontext"
	"backend/internal/security"
	"backend/internal/starterpolicy"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestValidateSelectedIP(t *testing.T) {
	const detected = "203.0.113.10"
	for _, tc := range []struct {
		name string
		body string
		code string
	}{
		{name: "automatic"},
		{name: "matching address", body: `{"clientIp":"203.0.113.10"}`},
		{name: "different address", body: `{"clientIp":"203.0.113.11"}`, code: "CLIENT_IP_MISMATCH"},
		{name: "IPv6", body: `{"clientIp":"2001:db8::1"}`, code: "INVALID_CLIENT_IP"},
		{name: "invalid", body: `{"clientIp":"203.0.113.999"}`, code: "INVALID_CLIENT_IP"},
		{name: "trailing JSON", body: `{"clientIp":"203.0.113.10"}{}`, code: "INVALID_CLIENT_IP"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			if code := validateSelectedIP(httptest.NewRecorder(), req, detected); code != tc.code {
				t.Fatalf("got %q, want %q", code, tc.code)
			}
		})
	}
}

func TestSavedIPLifecyclePostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	userID, otherID, planID := uuid.New(), uuid.New(), uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO users (id, failed_login_attempts, last_login_ip, email, full_name, email_verified)
		VALUES ($1, 181001, '192.0.2.158', $3, 'Saved IP test', true), ($2, 181002, '192.0.2.159', $4, 'Saved IP test', true)`,
		userID, otherID, userID.String()+"@example.invalid", otherID.String()+"@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM free_plan_claims WHERE user_id = $1", userID)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id IN ($1, $2)", userID, otherID)
		_, _ = pool.Exec(ctx, "DELETE FROM plans WHERE id = $1", planID)
	}()
	_, err = pool.Exec(ctx, `INSERT INTO plans (id, code, name, price_minor, currency, duration_days, data_limit_bytes, unlimited_bandwidth)
		VALUES ($1, $2, 'Saved IP test', 0, 'USD', 30, 0, true)`, planID, planID.String())
	if err != nil {
		t.Fatal(err)
	}
	store := starterpolicy.MarketFreeStore{DB: pool}
	if _, err := store.Claim(ctx, userID, planID.String()); err != nil {
		t.Fatal(err)
	}

	proxy, err := security.NewTrustedProxy(nil)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(NewService(NewRepository(pool)), proxy, "")
	call := func(handler http.HandlerFunc, user uuid.UUID, method, path, body, sourceIP string, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.RemoteAddr = sourceIP + ":1234"
		res := httptest.NewRecorder()
		handler(res, req.WithContext(requestcontext.WithUserID(ctx, user)))
		if res.Code != status {
			t.Fatalf("%s %s returned %d, want %d: %s", method, path, res.Code, status, res.Body)
		}
		return res
	}

	const first, second = "192.0.2.160", "192.0.2.161"
	call(h.Activate, userID, http.MethodPost, "/", "", first, http.StatusOK)
	call(h.Activate, userID, http.MethodPost, "/", `{"clientIp":"`+second+`"}`, first, http.StatusBadRequest)
	call(h.Activate, userID, http.MethodPost, "/", `{"clientIp":"`+second+`"}`, second, http.StatusOK)
	call(h.Activate, userID, http.MethodPost, "/", `{"clientIp":"`+second+`"}`, second, http.StatusOK)

	assertHistory := func(user uuid.UUID, expected ...string) {
		t.Helper()
		res := call(h.GetSavedIPs, user, http.MethodGet, "/", "", first, http.StatusOK)
		var history struct {
			IPs        []string
			DetectedIP string
		}
		if err := json.Unmarshal(res.Body.Bytes(), &history); err != nil {
			t.Fatal(err)
		}
		if history.DetectedIP != first {
			t.Fatalf("got detected IP %q, want %q", history.DetectedIP, first)
		}
		if history.IPs == nil || !slices.Equal(history.IPs, expected) {
			t.Fatalf("got history %v, want %v", history.IPs, expected)
		}
	}

	assertHistory(userID, second, first)
	assertHistory(otherID)
	call(h.DeleteSavedIP, otherID, http.MethodDelete, "/?clientIp="+second, "", first, http.StatusNoContent)
	assertHistory(userID, second, first)
	call(h.DeleteSavedIP, userID, http.MethodDelete, "/?clientIp="+second, "", first, http.StatusNoContent)
	assertHistory(userID, first)
	res := call(h.Current, userID, http.MethodGet, "/", "", second, http.StatusOK)
	var active Entitlement
	if err := json.Unmarshal(res.Body.Bytes(), &active); err != nil {
		t.Fatal(err)
	}
	if active.Status != "active" || active.ClientIP != second {
		t.Fatalf("deleting the saved address changed the active entitlement: %+v", active)
	}

	const remote = "192.0.2.162"
	requestApproval := func() ipApproval {
		res := call(h.CreateIPApproval, userID, http.MethodPost, "/", `{"clientIp":"`+remote+`"}`, first, http.StatusCreated)
		var approval ipApproval
		if err := json.Unmarshal(res.Body.Bytes(), &approval); err != nil {
			t.Fatal(err)
		}
		if approval.ID == uuid.Nil || approval.ClientIP != remote || !approval.ExpiresAt.After(time.Now()) {
			t.Fatalf("invalid approval response: %+v", approval)
		}
		return approval
	}
	confirm := func(approval ipApproval, account uuid.UUID, ip string, status int) {
		t.Helper()
		call(h.ConfirmIPApproval, account, http.MethodPost, "/", `{"id":"`+approval.ID.String()+`"}`, ip, status)
	}

	expired := requestApproval()
	if _, err := pool.Exec(ctx, "UPDATE network_ip_approvals SET expires_at = now() WHERE id = $1", expired.ID); err != nil {
		t.Fatal(err)
	}
	confirm(expired, userID, remote, http.StatusConflict)
	replaced := requestApproval()
	approval := requestApproval()
	confirm(replaced, userID, remote, http.StatusConflict)
	confirm(approval, otherID, remote, http.StatusConflict)
	confirm(approval, userID, first, http.StatusBadRequest)
	assertHistory(userID, first)
	confirm(approval, userID, remote, http.StatusOK)
	assertHistory(userID, remote, first)
	confirm(approval, userID, remote, http.StatusConflict)

}
