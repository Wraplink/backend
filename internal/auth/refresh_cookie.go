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
	setRememberDeviceCookie(w, false, time.Time{}, secure)
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

const rememberDeviceCookieName = "wraplink_remember_device"

// A session cookie is used unless the user explicitly opted into persistence.
func setRefreshCookieWithPersistence(w http.ResponseWriter, value string, expiresAt time.Time, secure, persistent bool) {
	cookie := &http.Cookie{
		Name: refreshCookieName, Value: value, Path: refreshCookiePath,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	}
	if persistent {
		cookie.Expires = expiresAt
		cookie.MaxAge = maxAgeFromExpiry(expiresAt)
	}
	http.SetCookie(w, cookie)
}

func setRememberDeviceCookie(w http.ResponseWriter, remember bool, expiresAt time.Time, secure bool) {
	cookie := &http.Cookie{
		Name: rememberDeviceCookieName, Path: refreshCookiePath,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	}
	if remember {
		cookie.Value = "1"
		cookie.Expires = expiresAt
		cookie.MaxAge = maxAgeFromExpiry(expiresAt)
	} else {
		cookie.MaxAge = -1
		cookie.Expires = time.Unix(1, 0)
	}
	http.SetCookie(w, cookie)
}
