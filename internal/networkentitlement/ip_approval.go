package networkentitlement

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"backend/internal/requestcontext"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	errIPApprovalUnavailable = errors.New("IP approval unavailable")
	errIPApprovalMismatch    = errors.New("approval IP mismatch")
)

type ipApproval struct {
	ID        uuid.UUID `json:"id"`
	ClientIP  string    `json:"clientIp"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (h *Handler) CreateIPApproval(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var input struct {
		ClientIP string `json:"clientIp"`
	}
	if !decodeApprovalRequest(w, r, &input) {
		return
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(input.ClientIP))
	if err != nil || !addr.Is4() || !isPublicIPv4(net.IP(addr.AsSlice())) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "INVALID_CLIENT_IP", "error": "Enter a public IPv4 address."})
		return
	}

	approval := ipApproval{ID: uuid.New(), ClientIP: addr.String()}
	err = h.service.repository.db.QueryRow(r.Context(), `
		INSERT INTO network_ip_approvals (user_id, id, client_ip, expires_at)
		VALUES ($1, $2, $3::inet, clock_timestamp() + interval '10 minutes')
		ON CONFLICT (user_id) DO UPDATE SET
			id = EXCLUDED.id, client_ip = EXCLUDED.client_ip,
			expires_at = clock_timestamp() + interval '10 minutes'
		RETURNING expires_at`, userID, approval.ID, approval.ClientIP).Scan(&approval.ExpiresAt)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not create IP approval")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, approval)
}

func (h *Handler) ConfirmIPApproval(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var input struct {
		ID uuid.UUID `json:"id"`
	}
	if !decodeApprovalRequest(w, r, &input) {
		return
	}
	if input.ID == uuid.Nil {
		writeJSONError(w, http.StatusBadRequest, "invalid IP approval ID")
		return
	}
	ip := h.trustedProxy.ClientIP(r)
	if !isPublicIPv4(ip) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "PUBLIC_IPV4_REQUIRED", "error": "A public IPv4 client address is required."})
		return
	}

	h.activate(w, r, userID, ip, input.ID)
}

func decodeApprovalRequest(w http.ResponseWriter, r *http.Request, input any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err == nil {
		var trailing any
		if decoder.Decode(&trailing) == io.EOF {
			return true
		}
	}
	writeJSONError(w, http.StatusBadRequest, "invalid IP approval request")
	return false
}

func consumeIPApproval(ctx context.Context, tx pgx.Tx, userID, approvalID uuid.UUID, detectedIP string) error {
	var requestedIP string
	err := tx.QueryRow(ctx, `DELETE FROM network_ip_approvals
		WHERE user_id = $1 AND id = $2 AND expires_at > clock_timestamp()
		RETURNING host(client_ip)`, userID, approvalID).Scan(&requestedIP)
	if errors.Is(err, pgx.ErrNoRows) {
		return errIPApprovalUnavailable
	}
	if err != nil {
		return err
	}
	if requestedIP != detectedIP {
		// The caller rolls back, retaining the approval for use from the correct IP.
		return errIPApprovalMismatch
	}
	return nil
}
