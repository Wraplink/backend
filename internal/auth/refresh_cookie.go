package auth

import (
	"net/http"
	"time"
)

const refreshCookieName = "wraplink_refresh"

const refreshCookiePath = "/api/v1/auth"

func setRefreshCookie(
	w http.ResponseWriter,
	value string,
	expiresAt time.Time,
	secure bool,
) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    value,
		Path:     refreshCookiePath,
		Expires:  expiresAt,
		MaxAge:   maxAgeFromExpiry(expiresAt),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearRefreshCookie(
	w http.ResponseWriter,
	secure bool,
) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		Expires:  time.Unix(1, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func maxAgeFromExpiry(expiresAt time.Time) int {
	seconds := int(time.Until(expiresAt).Seconds())

	if seconds <= 0 {
		return 0
	}

	return seconds
}
