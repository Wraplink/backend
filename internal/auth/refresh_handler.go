package auth

import (
	"errors"
	"net/http"
	"time"

	"backend/internal/security"
)

type RefreshHandler struct {
	service *RefreshService
}

func NewRefreshHandler(
	service *RefreshService,
) *RefreshHandler {
	return &RefreshHandler{
		service: service,
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
			clearRefreshCookie(w)

			writeJSONError(
				w,
				http.StatusUnauthorized,
				"authentication required",
			)

		case errors.Is(err, ErrInvalidRefreshToken):
			clearRefreshCookie(w)

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

	http.SetCookie(
		w,
		&http.Cookie{
			Name:     refreshTokenCookieName,
			Value:    result.RefreshToken,
			Path:     "/api/v1/auth",
			Domain:   h.service.config.RefreshCookieDomain,
			Expires:  result.RefreshTokenExpiry,
			MaxAge:   int(time.Until(result.RefreshTokenExpiry).Seconds()),
			HttpOnly: true,
			Secure:   h.service.config.SecureCookies,
			SameSite: http.SameSiteLaxMode,
		},
	)

	writeJSON(
		w,
		http.StatusOK,
		map[string]any{
			"accessToken": result.AccessToken,
		},
	)
}

func clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(
		w,
		&http.Cookie{
			Name:     refreshTokenCookieName,
			Value:    "",
			Path:     "/api/v1/auth",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   false,
			SameSite: http.SameSiteLaxMode,
		},
	)
}
