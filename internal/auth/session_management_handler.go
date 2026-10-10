package auth

import (
	"encoding/json"
	"errors"
	"net/http"

	"backend/internal/requestcontext"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type SessionManagementHandler struct {
	service *SessionManagementService
}

func NewSessionManagementHandler(
	service *SessionManagementService,
) *SessionManagementHandler {
	if service == nil {
		panic("auth: nil session management service")
	}

	return &SessionManagementHandler{service: service}
}

func (h *SessionManagementHandler) List(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	sessions, err := h.service.ListActive(r.Context(), userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not list sessions")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"sessions": sessions,
	}); err != nil {
		// The response may already have been partially written.
		return
	}
}

func (h *SessionManagementHandler) Revoke(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil || sessionID == uuid.Nil {
		writeJSONError(w, http.StatusBadRequest, "invalid session ID")
		return
	}

	err = h.service.Revoke(r.Context(), userID, sessionID)
	if err != nil {
		if errors.Is(err, ErrInvalidSessionID) {
			writeJSONError(w, http.StatusNotFound, "session not found")
			return
		}

		writeJSONError(w, http.StatusInternalServerError, "could not revoke session")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"message":"session revoked successfully"}`))
}
