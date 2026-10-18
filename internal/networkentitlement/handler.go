package networkentitlement

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"backend/internal/requestcontext"
	"backend/internal/security"
)

type Handler struct {
	service        *Service
	trustedProxy   *security.TrustedProxy
	controlSyncKey string
}

func NewHandler(
	service *Service,
	trustedProxy *security.TrustedProxy,
	controlSyncKey string,
) *Handler {
	if service == nil || trustedProxy == nil {
		panic("networkentitlement: nil handler dependency")
	}

	return &Handler{
		service:        service,
		trustedProxy:   trustedProxy,
		controlSyncKey: strings.TrimSpace(controlSyncKey),
	}
}

func (h *Handler) SnapshotEnabled() bool {
	return h.controlSyncKey != ""
}

func (h *Handler) Activate(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	clientIP := h.trustedProxy.ClientIP(r)
	if clientIP == nil || clientIP.To4() == nil {
		writeJSONError(w, http.StatusBadRequest, "a public IPv4 client address is required")
		return
	}

	result, err := h.service.Activate(
		r.Context(),
		userID,
		clientIP,
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrNoUsableSubscription):
			writeJSONError(w, http.StatusConflict, "an active subscription with remaining quota is required")
		case errors.Is(err, ErrClientIPInUse):
			writeJSONError(w, http.StatusConflict, "this public IP is already active on another account")
		case errors.Is(err, ErrInvalidClientIP):
			writeJSONError(w, http.StatusBadRequest, "invalid client IP")
		default:
			writeJSONError(w, http.StatusInternalServerError, "could not activate network entitlement")
		}
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) Current(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	result, err := h.service.Current(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrEntitlementNotFound) {
			writeJSONError(w, http.StatusNotFound, "network entitlement not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "could not get network entitlement")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// GetMode returns the selected per-user policy even when not connected.
func (h *Handler) GetMode(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	mode, err := h.service.GetMode(r.Context(), userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not read connection mode")
		return
	}
	writeJSON(w, http.StatusOK, ModeResponse{Mode: mode})
}

// SetMode accepts only an explicit known mode; never modifies global policy.
func (h *Handler) SetMode(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var request ModeResponse
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid connection mode request")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeJSONError(w, http.StatusBadRequest, "invalid connection mode request")
		return
	}
	if !request.Mode.Valid() {
		writeJSONError(w, http.StatusBadRequest, "mode must be smart, gaming, or streaming")
		return
	}
	if err := h.service.SetMode(r.Context(), userID, request.Mode); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not update connection mode")
		return
	}
	writeJSON(w, http.StatusOK, ModeResponse{Mode: request.Mode})
}

func (h *Handler) Deactivate(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	if err := h.service.Deactivate(r.Context(), userID); err != nil {
		if errors.Is(err, ErrEntitlementNotFound) {
			writeJSONError(w, http.StatusNotFound, "network entitlement not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "could not deactivate network entitlement")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Snapshot(
	w http.ResponseWriter,
	r *http.Request,
) {
	if !h.SnapshotEnabled() {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}

	provided := r.Header.Get("X-Network-Control-Key")
	if subtle.ConstantTimeCompare(
		[]byte(provided),
		[]byte(h.controlSyncKey),
	) != 1 {
		writeJSONError(w, http.StatusUnauthorized, "invalid network control key")
		return
	}

	result, err := h.service.Snapshot(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not build network entitlement snapshot")
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
