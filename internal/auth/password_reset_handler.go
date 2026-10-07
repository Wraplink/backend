package auth

import (
	"encoding/json"
	"net/http"
	"strings"
)

type passwordResetRequestHTTP struct {
	Email  string `json:"email"`
	Locale string `json:"locale"`
}

type passwordResetConfirmHTTP struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type PasswordResetHandler struct {
	service *PasswordResetService
}

func NewPasswordResetHandler(
	service *PasswordResetService,
) *PasswordResetHandler {
	return &PasswordResetHandler{
		service: service,
	}
}

func (h *PasswordResetHandler) Request(
	w http.ResponseWriter,
	r *http.Request,
) {
	r.Body = http.MaxBytesReader(
		w,
		r.Body,
		16*1024,
	)

	var req passwordResetRequestHTTP

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		http.Error(
			w,
			"invalid request",
			http.StatusBadRequest,
		)
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	req.Locale = strings.TrimSpace(req.Locale)

	if req.Locale == "" {
		req.Locale = "en"
	}

	err := h.service.Request(
		r.Context(),
		PasswordResetRequest{
			Email:     req.Email,
			Locale:    req.Locale,
			IPAddress: remoteIP(r.RemoteAddr),
			UserAgent: r.UserAgent(),
		},
	)

	if err != nil {
		http.Error(
			w,
			"internal server error",
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	w.WriteHeader(http.StatusAccepted)

	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": "If the account exists, you will receive a password reset email.",
	})
}

func (h *PasswordResetHandler) Confirm(
	w http.ResponseWriter,
	r *http.Request,
) {
	r.Body = http.MaxBytesReader(
		w,
		r.Body,
		16*1024,
	)

	var req passwordResetConfirmHTTP

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		http.Error(
			w,
			"invalid request",
			http.StatusBadRequest,
		)
		return
	}

	err := h.service.Confirm(
		r.Context(),
		PasswordResetConfirmRequest{
			Token:     req.Token,
			Password:  req.Password,
			IPAddress: remoteIP(r.RemoteAddr),
			UserAgent: r.UserAgent(),
		},
	)

	if err != nil {
		if err == ErrInvalidPasswordResetToken {
			http.Error(
				w,
				"invalid or expired password reset token",
				http.StatusBadRequest,
			)
			return
		}

		if err == ErrWeakPassword {
			http.Error(
				w,
				"invalid password",
				http.StatusBadRequest,
			)
			return
		}

		http.Error(
			w,
			"internal server error",
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": "Password has been reset successfully.",
	})
}
