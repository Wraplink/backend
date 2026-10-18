package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"backend/internal/networkaccess"
	"backend/internal/networkentitlement"
	"backend/internal/payments"
	"backend/internal/requestcontext"

	"github.com/google/uuid"
)

type walletService interface {
	Wallet(context.Context, uuid.UUID, string) (*payments.Wallet, error)
	Payments(context.Context, uuid.UUID, int) ([]payments.Payment, error)
}

type accessService interface {
	Access(context.Context, uuid.UUID) (*networkaccess.AccessInfo, error)
}

type entitlementService interface {
	Current(context.Context, uuid.UUID) (*networkentitlement.Entitlement, error)
}

type Handler struct {
	wallet      walletService
	access      accessService
	entitlement entitlementService
}

func NewHandler(wallet walletService, access accessService, entitlement entitlementService) *Handler {
	if wallet == nil || access == nil || entitlement == nil {
		panic("dashboard: nil service dependency")
	}
	return &Handler{wallet: wallet, access: access, entitlement: entitlement}
}

type Summary struct {
	Wallet         *payments.Wallet                `json:"wallet"`
	Access         *networkaccess.AccessInfo       `json:"access"`
	Entitlement    *networkentitlement.Entitlement `json:"entitlement"`
	RecentPayments []payments.Payment              `json:"recentPayments"`
}

// Summary returns real account information in one authenticated request.
// Missing subscriptions/entitlements are ordinary empty states, not failures.
func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	wallet, err := h.wallet.Wallet(r.Context(), userID, "IRR")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load dashboard")
		return
	}

	access, err := h.access.Access(r.Context(), userID)
	if errors.Is(err, networkaccess.ErrNoActiveSubscription) {
		access = nil
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load dashboard")
		return
	}

	entitlement, err := h.entitlement.Current(r.Context(), userID)
	if errors.Is(err, networkentitlement.ErrEntitlementNotFound) {
		entitlement = nil
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load dashboard")
		return
	}

	recentPayments, err := h.wallet.Payments(r.Context(), userID, 5)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load dashboard")
		return
	}
	if recentPayments == nil {
		recentPayments = []payments.Payment{}
	}
	writeJSON(w, http.StatusOK, Summary{
		Wallet: wallet, Access: access, Entitlement: entitlement, RecentPayments: recentPayments,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
