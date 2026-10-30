package networkaccess

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// IngestEdgeUsage accepts the edge-agent single-report protocol.
// Authentication uses the existing dedicated usage ingestion secret.
func (h *Handler) IngestEdgeUsage(w http.ResponseWriter, r *http.Request) {
	if !h.UsageIngestEnabled() {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	key := r.Header.Get("X-Network-Control-Key")
	if subtle.ConstantTimeCompare([]byte(key), []byte(h.usageIngestKey)) != 1 {
		writeJSONError(w, http.StatusUnauthorized, "invalid usage key")
		return
	}
	var report EdgeUsageReport
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeJSONError(w, http.StatusBadRequest, "unexpected trailing JSON data")
		return
	}
	if err := report.Validate(time.Now().UTC()); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid edge usage report")
		return
	}
	if strings.TrimSpace(r.Header.Get("Idempotency-Key")) != report.IdempotencyKey() {
		writeJSONError(w, http.StatusBadRequest, "invalid idempotency key")
		return
	}
	result, err := h.service.IngestEdgeUsage(r.Context(), report)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidEdgeUsage), errors.Is(err, ErrInvalidReport):
			writeJSONError(w, http.StatusBadRequest, "invalid edge usage report")
		case errors.Is(err, ErrSubscriptionNotFound):
			writeJSONError(w, http.StatusUnprocessableEntity, "no unambiguous entitlement for report interval")
		case errors.Is(err, ErrReportConflict):
			writeJSONError(w, http.StatusConflict, "conflicting edge report")
		case errors.Is(err, ErrSubscriptionNotUsable):
			writeJSONError(w, http.StatusConflict, "subscription cannot accept usage")
		default:
			writeJSONError(w, http.StatusInternalServerError, "could not ingest edge usage")
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}
