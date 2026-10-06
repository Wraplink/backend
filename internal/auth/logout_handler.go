package auth

import (
	"net/http"
)

type LogoutHandler struct {
	service      *LogoutService
	secureCookie bool
}

func NewLogoutHandler(
	service *LogoutService,
	secureCookie bool,
) *LogoutHandler {
	return &LogoutHandler{
		service:      service,
		secureCookie: secureCookie,
	}
}

func (h *LogoutHandler) Logout(
	w http.ResponseWriter,
	r *http.Request,
) {
	refreshCookie, err := r.Cookie(refreshCookieName)

	var refreshToken string

	if err == nil {
		refreshToken = refreshCookie.Value
	}

	_, err = h.service.Logout(
		r.Context(),
		refreshToken,
	)

	if err != nil {
		http.Error(
			w,
			"internal server error",
			http.StatusInternalServerError,
		)
		return
	}

	/*
	 * Always clear the cookie.
	 *
	 * This also makes logout idempotent when:
	 * - cookie does not exist
	 * - session was already revoked
	 * - session has expired
	 */
	clearRefreshCookie(
		w,
		h.secureCookie,
	)

	w.WriteHeader(http.StatusNoContent)
}
