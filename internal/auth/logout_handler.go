package auth

import (
	"errors"
	"net/http"
)

type LogoutHandler struct {
	sessions *SessionService
}

func NewLogoutHandler(
	sessions *SessionService,
) *LogoutHandler {
	return &LogoutHandler{
		sessions: sessions,
	}
}

func (h *LogoutHandler) Logout(
	w http.ResponseWriter,
	r *http.Request,
) {
	cookie, err := r.Cookie(refreshTokenCookieName)

	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			clearRefreshCookie(w)

			writeJSON(
				w,
				http.StatusNoContent,
				nil,
			)
			return
		}

		writeJSONError(
			w,
			http.StatusBadRequest,
			"invalid request",
		)
		return
	}

	if err := h.sessions.Revoke(
		r.Context(),
		cookie.Value,
	); err != nil {
		writeJSONError(
			w,
			http.StatusInternalServerError,
			"internal server error",
		)
		return
	}

	clearRefreshCookie(w)

	w.WriteHeader(http.StatusNoContent)
}
