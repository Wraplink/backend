package auth

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"

	"backend/internal/security"
)

type verifyEmailHTTPRequest struct {
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
		8*1024,
	)

	var request verifyEmailHTTPRequest

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

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
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

	ip := security.RemoteIP(r)

	var clientIP net.IP

	if ip != "" {
		clientIP = net.ParseIP(ip)
	}

	err := h.service.Verify(
		r.Context(),
		VerifyEmailRequest{
			Token:     request.Token,
			IPAddress: clientIP,
			UserAgent: r.UserAgent(),
		},
	)

	if err != nil {
		if errors.Is(err, ErrInvalidVerificationToken) {
			writeJSONError(
				w,
				http.StatusBadRequest,
				"invalid or expired verification token",
			)
			return
		}

		// Do not expose database/internal errors.
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
		map[string]string{
			"message": "email verified successfully",
		},
	)
}
