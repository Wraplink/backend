package networkaccess

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"backend/internal/requestcontext"
)

type Handler struct {
	service        *Service
	usageIngestKey string
}

func NewHandler(service *Service, usageIngestKey string) *Handler {
	if service == nil {
		panic("networkaccess: nil service")
	}

	return &Handler{
		service:        service,
		usageIngestKey: strings.TrimSpace(usageIngestKey),
	}
}

func (h *Handler) UsageIngestEnabled() bool {
	return h.usageIngestKey != ""
}

func (h *Handler) Access(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	result, err := h.service.Access(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrNoActiveSubscription) {
			writeJSONError(w, http.StatusNotFound, "no active subscription")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "could not get network access")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) Usage(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	days := 0
	if value := strings.TrimSpace(r.URL.Query().Get("days")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err == nil {
			days = parsed
		}
	}

	result, err := h.service.Usage(r.Context(), userID, days)
	if err != nil {
		if errors.Is(err, ErrNoActiveSubscription) {
			writeJSONError(w, http.StatusNotFound, "no active subscription")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "could not get usage")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) IngestUsage(w http.ResponseWriter, r *http.Request) {
	if !h.UsageIngestEnabled() {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}

	provided := r.Header.Get("X-Usage-Key")
	if subtle.ConstantTimeCompare([]byte(provided), []byte(h.usageIngestKey)) != 1 {
		writeJSONError(w, http.StatusUnauthorized, "invalid usage key")
		return
	}

	var request IngestRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.service.Ingest(r.Context(), request)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidReport):
			writeJSONError(w, http.StatusBadRequest, "invalid usage report")
		case errors.Is(err, ErrSubscriptionNotFound):
			writeJSONError(w, http.StatusNotFound, "subscription not found")
		case errors.Is(err, ErrSubscriptionNotUsable):
			writeJSONError(w, http.StatusConflict, "subscription cannot accept usage reports")
		case errors.Is(err, ErrReportConflict):
			writeJSONError(w, http.StatusConflict, "reportId was already used with different usage data")
		default:
			writeJSONError(w, http.StatusInternalServerError, "could not ingest usage")
		}
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
