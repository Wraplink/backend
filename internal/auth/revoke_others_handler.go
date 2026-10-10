package auth

import (
	"backend/internal/requestcontext"
	"net/http"
)

type RevokeOthersHandler struct {
	service *RevokeOthersService
}

func NewRevokeOthersHandler(
	service *RevokeOthersService,
) *RevokeOthersHandler {
	return &RevokeOthersHandler{
		service: service,
	}
}

func (h *RevokeOthersHandler) Revoke(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(
			w,
			http.StatusUnauthorized,
			"authentication required",
		)
		return
	}

	sessionID, ok := requestcontext.SessionID(r.Context())
	if !ok {
		writeJSONError(
			w,
			http.StatusUnauthorized,
			"authentication required",
		)
		return
	}

	count, err := h.service.Revoke(
		r.Context(),
		userID,
		sessionID,
	)
	if err != nil {
		writeJSONError(
			w,
			http.StatusInternalServerError,
			"internal server error",
		)
		return
	}

	writeJSON(
		w,
		http.StatusOK,
		map[string]any{
			"message":         "other sessions revoked",
			"revokedSessions": count,
		},
	)
}
