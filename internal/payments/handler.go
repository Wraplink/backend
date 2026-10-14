package payments

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"backend/internal/requestcontext"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	if service == nil {
		panic("payments: nil service")
	}

	return &Handler{service: service}
}

func (h *Handler) Wallet(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	wallet, err := h.service.Wallet(r.Context(), userID, r.URL.Query().Get("currency"))
	if err != nil {
		if errors.Is(err, ErrInvalidCurrency) {
			writeJSONError(w, http.StatusBadRequest, "currency must be a three-letter code")
			return
		}

		writeJSONError(w, http.StatusInternalServerError, "could not get wallet")
		return
	}

	writeJSON(w, http.StatusOK, wallet)
}

func (h *Handler) Transactions(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	items, err := h.service.Transactions(
		r.Context(),
		userID,
		r.URL.Query().Get("currency"),
		queryLimit(r),
	)
	if err != nil {
		if errors.Is(err, ErrInvalidCurrency) {
			writeJSONError(w, http.StatusBadRequest, "currency must be a three-letter code")
			return
		}

		writeJSONError(w, http.StatusInternalServerError, "could not list wallet transactions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"transactions": items,
	})
}

func (h *Handler) Payments(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	items, err := h.service.Payments(r.Context(), userID, queryLimit(r))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not list payments")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"payments": items,
	})
}

func (h *Handler) Payment(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	paymentID, err := uuid.Parse(chi.URLParam(r, "paymentID"))
	if err != nil || paymentID == uuid.Nil {
		writeJSONError(w, http.StatusBadRequest, "invalid payment ID")
		return
	}

	payment, err := h.service.Payment(r.Context(), userID, paymentID)
	if err != nil {
		if errors.Is(err, ErrPaymentNotFound) {
			writeJSONError(w, http.StatusNotFound, "payment not found")
			return
		}

		writeJSONError(w, http.StatusInternalServerError, "could not get payment")
		return
	}

	writeJSON(w, http.StatusOK, payment)
}

func (h *Handler) PaySubscriptionWithWallet(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	subscriptionID, err := uuid.Parse(chi.URLParam(r, "subscriptionID"))
	if err != nil || subscriptionID == uuid.Nil {
		writeJSONError(w, http.StatusBadRequest, "invalid subscription ID")
		return
	}

	result, err := h.service.PaySubscriptionWithWallet(
		r.Context(),
		userID,
		subscriptionID,
		r.Header.Get("Idempotency-Key"),
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidIdempotencyKey):
			writeJSONError(w, http.StatusBadRequest, "Idempotency-Key header is required and must be at most 100 characters")
		case errors.Is(err, ErrSubscriptionNotFound):
			writeJSONError(w, http.StatusNotFound, "subscription not found")
		case errors.Is(err, ErrSubscriptionNotPending):
			writeJSONError(w, http.StatusConflict, "subscription is not pending payment")
		case errors.Is(err, ErrActiveSubscriptionExists):
			writeJSONError(w, http.StatusConflict, "an active subscription already exists")
		case errors.Is(err, ErrInsufficientWalletBalance):
			writeJSONError(w, http.StatusConflict, "insufficient wallet balance")
		case errors.Is(err, ErrIdempotencyConflict):
			writeJSONError(w, http.StatusConflict, "idempotency key was already used for another payment")
		default:
			writeJSONError(w, http.StatusInternalServerError, "could not pay subscription")
		}
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func authenticatedUserID(
	w http.ResponseWriter,
	r *http.Request,
) (uuid.UUID, bool) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return uuid.Nil, false
	}

	return userID, true
}

func queryLimit(r *http.Request) int {
	value := strings.TrimSpace(r.URL.Query().Get("limit"))
	if value == "" {
		return 0
	}

	limit, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}

	return limit
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
