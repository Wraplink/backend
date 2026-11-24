package networkentitlement

import (
	"context"
	"net/http"
	"net/netip"

	"backend/internal/requestcontext"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (h *Handler) GetSavedIPs(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	ips, err := h.service.repository.getSavedIPs(r.Context(), userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not load saved IPs")
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"ips": ips})
}

func (h *Handler) DeleteSavedIP(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	addr, err := netip.ParseAddr(r.URL.Query().Get("clientIp"))
	if err != nil || !addr.Is4() {
		writeJSONError(w, http.StatusBadRequest, "invalid IPv4 address")
		return
	}
	if err := h.service.repository.deleteSavedIP(r.Context(), userID, addr.String()); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not delete saved IP")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (r *Repository) getSavedIPs(ctx context.Context, userID uuid.UUID) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT host(client_ip) FROM network_saved_ips
		WHERE user_id = $1 ORDER BY last_used_at DESC, client_ip`, userID)
	if err != nil {
		return nil, err
	}
	ips, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if ips == nil {
		ips = []string{}
	}
	return ips, err
}

func (r *Repository) deleteSavedIP(ctx context.Context, userID uuid.UUID, ip string) error {
	_, err := r.db.Exec(ctx, "DELETE FROM network_saved_ips WHERE user_id = $1 AND client_ip = $2::inet", userID, ip)
	return err
}
