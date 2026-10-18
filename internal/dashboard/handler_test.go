package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/networkaccess"
	"backend/internal/networkentitlement"
	"backend/internal/payments"
	"backend/internal/requestcontext"

	"github.com/google/uuid"
)

type fakeWallet struct{}

func (fakeWallet) Wallet(_ context.Context, _ uuid.UUID, _ string) (*payments.Wallet, error) {
	return &payments.Wallet{Currency: "IRR", BalanceMinor: 5000}, nil
}

func (fakeWallet) Payments(_ context.Context, _ uuid.UUID, limit int) ([]payments.Payment, error) {
	if limit != 5 {
		panic("expected recent payments limit 5")
	}
	return nil, nil
}

type fakeAccess struct{}

func (fakeAccess) Access(context.Context, uuid.UUID) (*networkaccess.AccessInfo, error) {
	return nil, networkaccess.ErrNoActiveSubscription
}

type fakeEntitlement struct{}

func (fakeEntitlement) Current(context.Context, uuid.UUID) (*networkentitlement.Entitlement, error) {
	return nil, networkentitlement.ErrEntitlementNotFound
}

func TestSummaryRequiresAuthentication(t *testing.T) {
	handler := NewHandler(fakeWallet{}, fakeAccess{}, fakeEntitlement{})
	resp := httptest.NewRecorder()
	handler.Summary(resp, httptest.NewRequest(http.MethodGet, "/api/v1/me/dashboard", nil))
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", resp.Code)
	}
}

func TestSummaryHasHonestEmptyState(t *testing.T) {
	handler := NewHandler(fakeWallet{}, fakeAccess{}, fakeEntitlement{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/dashboard", nil)
	req = req.WithContext(requestcontext.WithUserID(req.Context(), uuid.New()))
	resp := httptest.NewRecorder()
	handler.Summary(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("got %d, want 200: %s", resp.Code, resp.Body.String())
	}
	var decoded struct {
		Wallet         *payments.Wallet                `json:"wallet"`
		Access         *networkaccess.AccessInfo       `json:"access"`
		Entitlement    *networkentitlement.Entitlement `json:"entitlement"`
		RecentPayments []payments.Payment              `json:"recentPayments"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Wallet == nil || decoded.Wallet.BalanceMinor != 5000 {
		t.Fatalf("unexpected wallet: %+v", decoded.Wallet)
	}
	if decoded.Access != nil || decoded.Entitlement != nil || decoded.RecentPayments == nil {
		t.Fatalf("expected nil access/entitlement and empty payments: %+v", decoded)
	}
}
