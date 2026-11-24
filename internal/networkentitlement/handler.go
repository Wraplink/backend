package networkentitlement

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"backend/internal/requestcontext"
	"backend/internal/security"

	"github.com/google/uuid"
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
	if !isPublicIPv4(clientIP) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "PUBLIC_IPV4_REQUIRED", "error": "A public IPv4 client address is required."})
		return
	}

	if code := validateSelectedIP(w, r, clientIP.String()); code != "" {
		message := "Enter a valid IPv4 address."
		if code == "CLIENT_IP_MISMATCH" {
			message = "The selected IP does not match the current connection."
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": code, "error": message})
		return
	}

	h.activate(w, r, userID, clientIP, uuid.Nil)
}

func isPublicIPv4(ip net.IP) bool {
	return ip.To4() != nil && ip.IsGlobalUnicast() && !ip.IsPrivate()
}

func (h *Handler) activate(w http.ResponseWriter, r *http.Request, userID uuid.UUID, clientIP net.IP, approvalID uuid.UUID) {
	result, err := h.service.Activate(
		r.Context(),
		userID,
		clientIP,
		approvalID,
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrNoUsableSubscription):
			writeJSON(w, http.StatusConflict, map[string]string{"code": "NO_USABLE_SUBSCRIPTION", "error": "No active plan with remaining bandwidth. Purchase another plan to connect."})
		case errors.Is(err, ErrClientIPInUse):
			writeJSON(w, http.StatusConflict, map[string]string{"code": "CLIENT_IP_IN_USE", "error": "This public IP is already active on another account."})
		case errors.Is(err, errIPApprovalUnavailable):
			writeJSON(w, http.StatusConflict, map[string]string{"code": "IP_APPROVAL_UNAVAILABLE", "error": "This approval is unavailable or expired."})
		case errors.Is(err, errIPApprovalMismatch):
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "CLIENT_IP_MISMATCH", "error": "Approve this request from the requested network."})
		case errors.Is(err, ErrInvalidClientIP):
			writeJSONError(w, http.StatusBadRequest, "invalid client IP")
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"code": "CONNECTION_FAILED", "error": "Connection failed. Please try again or contact support."})
		}
		return
	}

	h.recordEvent(r.Context(), userID, "success", "Connection activated")
	writeJSON(w, http.StatusOK, result)
}

// An explicit choice must match the address observed by the trusted proxy.
func validateSelectedIP(w http.ResponseWriter, r *http.Request, detected string) string {
	if r.Body == nil {
		return ""
	}
	var request struct {
		ClientIP string `json:"clientIp"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err == io.EOF {
		return ""
	} else if err != nil {
		return "INVALID_CLIENT_IP"
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return "INVALID_CLIENT_IP"
	}
	if request.ClientIP == "" {
		return ""
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(request.ClientIP))
	if err != nil || !addr.Is4() {
		return "INVALID_CLIENT_IP"
	}
	if addr.String() != detected {
		return "CLIENT_IP_MISMATCH"
	}
	return ""
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
	h.recordEvent(r.Context(), userID, "info", "Connection mode changed to "+string(request.Mode))
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

	h.recordEvent(r.Context(), userID, "info", "Connection disconnected")
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
