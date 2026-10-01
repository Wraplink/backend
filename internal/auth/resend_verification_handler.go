package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"backend/internal/security"
)

type resendVerificationHTTPRequest struct {
	Email string `json:"email"`
}

type ResendVerificationHandler struct {
	service      *ResendVerificationService
	emailLimiter *security.RateLimiter
}

func NewResendVerificationHandler(
	service *ResendVerificationService,
	emailLimiter *security.RateLimiter,
) *ResendVerificationHandler {
	return &ResendVerificationHandler{
		service:      service,
		emailLimiter: emailLimiter,
	}
}

func (h *ResendVerificationHandler) Resend(
	w http.ResponseWriter,
	r *http.Request,
) {
	r.Body = http.MaxBytesReader(
		w,
		r.Body,
		4*1024,
	)

	var request resendVerificationHTTPRequest

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

	if request.Email == "" {
		writeJSONError(
			w,
			http.StatusBadRequest,
			"invalid request",
		)
		return
	}

	request.Email = strings.TrimSpace(request.Email)

	emailKey := strings.ToLower(request.Email)

	if !h.emailLimiter.Allow(emailKey) {
		w.Header().Set(
			"Retry-After",
			"3600",
		)

		writeJSONError(
			w,
			http.StatusTooManyRequests,
			"too many requests",
		)
		return
	}

	ip := security.RemoteIP(r)

	result, err := h.service.Resend(
		r.Context(),
		request.Email,
		parseIP(ip),
		r.UserAgent(),
	)

	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidResendRequest):
			/*
				Deliberately generic.

				We don't tell the caller whether:
				- the email exists
				- the email is verified
				- the account is pending
			*/
			writeJSON(
				w,
				http.StatusAccepted,
				map[string]string{
					"message": "if the account requires verification, a verification email will be sent",
				},
			)
			return

		default:
			writeJSONError(
				w,
				http.StatusInternalServerError,
				"internal server error",
			)
			return
		}
	}

	/*
		BACKEND TODO:
		Send verification email.

		The raw token exists only in result.Token and must never
		be persisted or logged.

		Example URL:

		https://wraplink.ir/auth/verify-email?token=<result.Token>
	*/

	_ = result

	writeJSON(
		w,
		http.StatusAccepted,
		map[string]string{
			"message": "if the account requires verification, a verification email will be sent",
		},
	)
}
