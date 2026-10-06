package auth

import (
	"backend/internal/security"
	"errors"
	"net/http"
)

type RefreshHandler struct {
	service      *RefreshService
	secureCookie bool
}

func NewRefreshHandler(
	service *RefreshService,
	secureCookie bool,
) *RefreshHandler {
	return &RefreshHandler{
		service:      service,
		secureCookie: secureCookie,
	}
}

func (h *RefreshHandler) Refresh(
	w http.ResponseWriter,
	r *http.Request,
) {
	cookie, err := r.Cookie(refreshTokenCookieName)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			writeJSONError(
				w,
				http.StatusUnauthorized,
				"authentication required",
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

	if cookie.Value == "" {
		writeJSONError(
			w,
			http.StatusUnauthorized,
			"authentication required",
		)
		return
	}

	ip := security.RemoteIP(r)

	result, err := h.service.Refresh(
		r.Context(),
		cookie.Value,
		parseIP(ip),
		r.UserAgent(),
	)

	if err != nil {
		switch {
		case errors.Is(err, ErrRefreshTokenReplay):
			/*
				Do not reveal details to the client.

				The session family has already been revoked.
			*/
			clearRefreshCookie(w, h.secureCookie)

			writeJSONError(
				w,
				http.StatusUnauthorized,
				"authentication required",
			)

		case errors.Is(err, ErrInvalidRefreshToken):
			clearRefreshCookie(w, h.secureCookie)

			writeJSONError(
				w,
				http.StatusUnauthorized,
				"authentication required",
			)

		default:
			writeJSONError(
				w,
				http.StatusInternalServerError,
				"internal server error",
			)
		}

		return
	}

	setRefreshCookie(w, result.RefreshToken, result.RefreshTokenExpiry, h.service.config.SecureCookies)

	writeJSON(
		w,
		http.StatusOK,
		map[string]any{
			"accessToken": result.AccessToken,
		},
	)
}
