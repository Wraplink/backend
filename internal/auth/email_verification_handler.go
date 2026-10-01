package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type verifyEmailRequest struct {
	Token string `json:"token"`
}

type EmailVerificationHandler struct {
	service *EmailVerificationService
}

func NewEmailVerificationHandler(
	service *EmailVerificationService,
) *EmailVerificationHandler {
	return &EmailVerificationHandler{
		service: service,
	}
}

func (h *EmailVerificationHandler) Verify(
	w http.ResponseWriter,
	r *http.Request,
) {
	r.Body = http.MaxBytesReader(
		w,
		r.Body,
		4*1024,
	)

	var request verifyEmailRequest

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

	request.Token = strings.TrimSpace(request.Token)

	if request.Token == "" {
		writeJSONError(
			w,
			http.StatusBadRequest,
			"invalid verification token",
		)
		return
	}

	if err := h.service.Verify(
		r.Context(),
		request.Token,
	); err != nil {
		switch {
		case errors.Is(err, ErrInvalidVerificationToken):
			writeJSONError(
				w,
				http.StatusBadRequest,
				"invalid or expired verification token",
			)

		case errors.Is(err, ErrVerificationExpired):
			writeJSONError(
				w,
				http.StatusBadRequest,
				"invalid or expired verification token",
			)

		case errors.Is(err, ErrVerificationUsed):
			writeJSONError(
				w,
				http.StatusBadRequest,
				"verification token already used",
			)

		case errors.Is(err, ErrEmailAlreadyVerified):
			writeJSONError(
				w,
				http.StatusConflict,
				"email already verified",
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

	writeJSON(
		w,
		http.StatusOK,
		map[string]string{
			"message": "email verified successfully",
		},
	)
}
