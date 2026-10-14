package payments

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type DepositHandler struct {
	service                      *Service
	developmentSettlementKey     string
	developmentSettlementEnabled bool
}

func NewDepositHandler(
	service *Service,
	developmentSettlementKey string,
	developmentSettlementEnabled bool,
) *DepositHandler {
	if service == nil {
		panic("payments: nil deposit service")
	}

	key := strings.TrimSpace(developmentSettlementKey)
	return &DepositHandler{
		service:                      service,
		developmentSettlementKey:     key,
		developmentSettlementEnabled: developmentSettlementEnabled && key != "",
	}
}

func (h *DepositHandler) DevelopmentSettlementEnabled() bool {
	return h.developmentSettlementEnabled
}

func (h *DepositHandler) Create(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	var request struct {
		AmountMinor int64  `json:"amountMinor"`
		Currency    string `json:"currency"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	deposit, err := h.service.CreateDeposit(
		r.Context(),
		userID,
		request.AmountMinor,
		request.Currency,
		r.Header.Get("Idempotency-Key"),
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidDepositAmount):
			writeJSONError(w, http.StatusBadRequest, "amountMinor must be greater than zero")
		case errors.Is(err, ErrInvalidCurrency):
			writeJSONError(w, http.StatusBadRequest, "currency must be a three-letter code")
		case errors.Is(err, ErrInvalidIdempotencyKey):
			writeJSONError(w, http.StatusBadRequest, "Idempotency-Key header is required and must be at most 100 characters")
		case errors.Is(err, ErrIdempotencyConflict):
			writeJSONError(w, http.StatusConflict, "idempotency key was already used with different deposit details")
		default:
			writeJSONError(w, http.StatusInternalServerError, "could not create deposit")
		}
		return
	}

	writeJSON(w, http.StatusCreated, deposit)
}

func (h *DepositHandler) List(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	deposits, err := h.service.Deposits(r.Context(), userID, queryLimit(r))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not list deposits")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"deposits": deposits})
}

func (h *DepositHandler) Get(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := authenticatedUserID(w, r)
	if !ok {
		return
	}

	depositID, err := uuid.Parse(chi.URLParam(r, "depositID"))
	if err != nil || depositID == uuid.Nil {
		writeJSONError(w, http.StatusBadRequest, "invalid deposit ID")
		return
	}

	deposit, err := h.service.Deposit(r.Context(), userID, depositID)
	if err != nil {
		if errors.Is(err, ErrDepositNotFound) {
			writeJSONError(w, http.StatusNotFound, "deposit not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "could not get deposit")
		return
	}

	writeJSON(w, http.StatusOK, deposit)
}

func (h *DepositHandler) SettleDevelopment(
	w http.ResponseWriter,
	r *http.Request,
) {
	if !h.developmentSettlementEnabled {
		http.NotFound(w, r)
		return
	}

	providedKey := r.Header.Get("X-Settlement-Key")
	if subtle.ConstantTimeCompare(
		[]byte(providedKey),
		[]byte(h.developmentSettlementKey),
	) != 1 {
		writeJSONError(w, http.StatusUnauthorized, "invalid settlement key")
		return
	}

	depositID, err := uuid.Parse(chi.URLParam(r, "depositID"))
	if err != nil || depositID == uuid.Nil {
		writeJSONError(w, http.StatusBadRequest, "invalid deposit ID")
		return
	}

	var request struct {
		ProviderReference string `json:"providerReference"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.service.SettleDevelopmentDeposit(
		r.Context(),
		depositID,
		request.ProviderReference,
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidProviderReference):
			writeJSONError(w, http.StatusBadRequest, "providerReference is required and must be at most 200 characters")
		case errors.Is(err, ErrDepositNotFound):
			writeJSONError(w, http.StatusNotFound, "deposit not found")
		case errors.Is(err, ErrDepositNotPending):
			writeJSONError(w, http.StatusConflict, "deposit is not pending")
		case errors.Is(err, ErrIdempotencyConflict):
			writeJSONError(w, http.StatusConflict, "provider reference conflicts with an existing settlement")
		default:
			writeJSONError(w, http.StatusInternalServerError, "could not settle deposit")
		}
		return
	}

	writeJSON(w, http.StatusOK, result)
}
