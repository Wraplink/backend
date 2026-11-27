package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"backend/internal/password"
	"backend/internal/requestcontext"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errAccountDeletionUnavailable = errors.New("account cannot be deleted")

type AccountDeleteHandler struct {
	db           *pgxpool.Pool
	secureCookie bool
}

func NewAccountDeleteHandler(db *pgxpool.Pool, secureCookie bool) *AccountDeleteHandler {
	return &AccountDeleteHandler{db: db, secureCookie: secureCookie}
}

func (h *AccountDeleteHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
	decoder.DisallowUnknownFields()
	var body struct {
		CurrentPassword   string `json:"currentPassword"`
		ConfirmationEmail string `json:"confirmationEmail"`
	}
	if err := decoder.Decode(&body); err != nil || len(body.CurrentPassword) > 128 || len(body.ConfirmationEmail) > 254 ||
		(body.CurrentPassword == "") == (body.ConfirmationEmail == "") {
		writeJSONError(w, http.StatusBadRequest, "password or email confirmation is required")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	switch err := h.deleteAccount(r.Context(), userID, body.CurrentPassword, body.ConfirmationEmail); {
	case errors.Is(err, ErrInvalidCurrentPassword):
		writeJSONError(w, http.StatusForbidden, "account confirmation is incorrect")
	case errors.Is(err, errAccountDeletionUnavailable):
		writeJSONError(w, http.StatusConflict, "account cannot be deleted")
	case err != nil:
		slog.Error("account deletion failed", "user_id", userID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "could not delete account")
	default:
		clearRefreshCookie(w, h.secureCookie)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *AccountDeleteHandler) deleteAccount(ctx context.Context, userID uuid.UUID, currentPassword, confirmationEmail string) error {
	var hash, email string
	var hasGoogleIdentity bool
	err := h.db.QueryRow(ctx, `SELECT u.email, COALESCE(c.password_hash, ''),
		EXISTS (SELECT 1 FROM google_identities WHERE user_id = u.id)
		FROM users u LEFT JOIN user_credentials c ON c.user_id = u.id
		WHERE u.id = $1`, userID).Scan(&email, &hash, &hasGoogleIdentity)
	if errors.Is(err, pgx.ErrNoRows) {
		return errAccountDeletionUnavailable
	}
	if err != nil {
		return fmt.Errorf("load account confirmation details: %w", err)
	}
	if hash != "" {
		if !password.Verify(currentPassword, hash) {
			return ErrInvalidCurrentPassword
		}
	} else if !hasGoogleIdentity || !strings.EqualFold(strings.TrimSpace(confirmationEmail), email) {
		return ErrInvalidCurrentPassword
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if hash != "" {
		// Recheck the verified hash so a concurrent password change cannot
		// authorize deletion with an old password.
		result, err := tx.Exec(ctx, "DELETE FROM user_credentials WHERE user_id = $1 AND password_hash = $2", userID, hash)
		if err != nil {
			return fmt.Errorf("remove password credentials: %w", err)
		}
		if result.RowsAffected() != 1 {
			return ErrInvalidCurrentPassword
		}

	}

	if _, err := tx.Exec(ctx, `UPDATE user_sessions SET revoked_at = now(), revoked_reason = 'logout'
		WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
		return fmt.Errorf("revoke account sessions: %w", err)
	}

	result, err := tx.Exec(ctx, `UPDATE users SET account_status = 'deleted', updated_at = now()
		WHERE id = $1 AND account_status = 'active'
		  AND NOT EXISTS (SELECT 1 FROM access_admins WHERE user_id = $1)
		  AND NOT EXISTS (SELECT 1 FROM user_credentials WHERE user_id = $1)
		  AND ($2 OR (email = $3 AND EXISTS (SELECT 1 FROM google_identities WHERE user_id = $1)))`,
		userID, hash != "", email)
	if err != nil {
		return fmt.Errorf("mark account deleted: %w", err)
	}
	if result.RowsAffected() != 1 {
		return errAccountDeletionUnavailable
	}

	if _, err := tx.Exec(ctx, `UPDATE network_entitlements
		SET status = 'revoked', revoked_at = now(), updated_at = now()
		WHERE user_id = $1 AND status = 'active'`, userID); err != nil {
		return fmt.Errorf("revoke network entitlements: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO security_events (user_id, event_type) VALUES ($1, 'ACCOUNT_DELETED')", userID); err != nil {
		return fmt.Errorf("record account deletion: %w", err)
	}
	return tx.Commit(ctx)
}
