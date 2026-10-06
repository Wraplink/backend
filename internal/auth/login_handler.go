package auth

import (
	"backend/internal/security"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
)

const refreshTokenCookieName = "wraplink_refresh_token"

type loginHTTPRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginHandler struct {
	service      *LoginService
	secureCookie bool
}

func NewLoginHandler(
	service *LoginService,
	secureCookie bool,

) *LoginHandler {
	return &LoginHandler{
		service:      service,
		secureCookie: secureCookie,
	}
}

func (h *LoginHandler) Login(
	w http.ResponseWriter,
	r *http.Request,
) {
	r.Body = http.MaxBytesReader(
		w,
		r.Body,
		16*1024,
	)

	var request loginHTTPRequest

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&request); err != nil {
		writeJSONError(
			w,
			http.StatusBadRequest,
			"invalid request",
		)
		return
	}

	// Reject trailing JSON/data.
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSONError(
			w,
			http.StatusBadRequest,
			"invalid request",
		)
		return
	}

	request.Email = strings.TrimSpace(request.Email)

	if request.Email == "" ||
		request.Password == "" {
		writeJSONError(
			w,
			http.StatusBadRequest,
			"invalid credentials",
		)
		return
	}

	ip := security.RemoteIP(r)

	result, err := h.service.Login(
		r.Context(),
		LoginRequest{
			Email:     request.Email,
			Password:  request.Password,
			IPAddress: parseIP(ip),
			UserAgent: r.UserAgent(),
		},
	)

	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials):
			// Deliberately generic.
			writeJSONError(
				w,
				http.StatusUnauthorized,
				"invalid email or password",
			)

		case errors.Is(err, ErrAccountLocked):
			writeJSONError(
				w,
				http.StatusTooManyRequests,
				"account temporarily locked",
			)

		case errors.Is(err, ErrEmailNotVerified):
			writeJSONError(
				w,
				http.StatusForbidden,
				"email verification required",
			)

		case errors.Is(err, ErrAccountDisabled):
			writeJSONError(
				w,
				http.StatusForbidden,
				"account unavailable",
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

	/*
		Refresh token security:

		- Never return refresh token in JSON.
		- Never store refresh token in localStorage.
		- Browser receives it only through HttpOnly cookie.
		- Database stores only SHA-256 hash.
	*/
	setRefreshCookie(w, result.RefreshToken, result.RefreshUntil, h.secureCookie)

	writeJSON(
		w,
		http.StatusOK,
		map[string]any{
			"accessToken": result.AccessToken,
			"user": map[string]any{
				"id":    result.UserID,
				"name":  result.FullName,
				"email": result.Email,
			},
		},
	)
}

func parseIP(value string) net.IP {
	return net.ParseIP(value)
}
