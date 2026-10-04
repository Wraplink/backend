package auth

import (
	"backend/internal/legal"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
)

const maxRegisterBodySize = 16 * 1024

type registerLegalRequest struct {
	TermsVersion     string `json:"termsVersion"`
	PrivacyVersion   string `json:"privacyVersion"`
	MarketingConsent bool   `json:"marketingConsent"`
}

type registerHTTPRequest struct {
	FullName string               `json:"name"`
	Email    string               `json:"email"`
	Password string               `json:"password"`
	Legal    registerLegalRequest `json:"legal"`
	Locale   string               `json:"locale"`
}

type registerHTTPResponse struct {
	Message string `json:"message"`
}

type RegisterHandler struct {
	service *RegistrationService
}

func NewRegisterHandler(service *RegistrationService) *RegisterHandler {
	return &RegisterHandler{
		service: service,
	}
}

func (h *RegisterHandler) Register(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(
		w,
		r.Body,
		maxRegisterBodySize,
	)

	var request registerHTTPRequest

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

	if decoder.Decode(&struct{}{}) != io.EOF {
		writeJSONError(
			w,
			http.StatusBadRequest,
			"invalid request",
		)
		return
	}

	request.FullName = strings.TrimSpace(request.FullName)
	request.Email = strings.TrimSpace(request.Email)

	if err := validateRegistrationInput(
		request.FullName,
		request.Email,
		request.Password,
	); err != nil {
		writeJSONError(
			w,
			http.StatusBadRequest,
			"invalid registration data",
		)
		return
	}

	if request.Legal.TermsVersion != legal.CurrentTermsVersion ||
		request.Legal.PrivacyVersion != legal.CurrentPrivacyVersion {
		writeJSONError(
			w,
			http.StatusBadRequest,
			"you must accept the current terms and privacy policy",
		)
		return
	}

	ipAddress := remoteIP(r.RemoteAddr)

	result, err := h.service.Register(
		r.Context(),
		RegisterRequest{
			FullName:         request.FullName,
			Email:            request.Email,
			Password:         request.Password,
			TermsVersion:     request.Legal.TermsVersion,
			PrivacyVersion:   request.Legal.PrivacyVersion,
			MarketingConsent: request.Legal.MarketingConsent,
			IPAddress:        ipAddress,
			UserAgent:        r.UserAgent(),
		},
	)

	if err != nil {
		if errors.Is(err, ErrEmailAlreadyExists) {
			// Deliberately return the same response as successful
			// registration to prevent account/email enumeration.
			writeJSON(
				w,
				http.StatusAccepted,
				registerHTTPResponse{
					Message: "If the registration can be completed, you will receive an email with further instructions.",
				},
			)
			return
		}

		if errors.Is(err, ErrInvalidRegistration) {
			writeJSONError(
				w,
				http.StatusBadRequest,
				"invalid registration data",
			)
			return
		}

		// Do not expose internal/database errors.
		http.Error(
			w,
			`{"error":"internal server error"}`,
			http.StatusInternalServerError,
		)

		return
	}

	// IMPORTANT:
	// Do not return the verification token to the browser.
	//
	// BACKEND TODO:
	// Send result.EmailVerification to the email service here,
	// after the database transaction has committed.

	_ = result

	writeJSON(
		w,
		http.StatusAccepted,
		registerHTTPResponse{
			Message: "If the registration can be completed, you will receive an email with further instructions.",
		},
	)
}

func remoteIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return nil
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return nil
	}

	return ip
}

func writeJSON(
	w http.ResponseWriter,
	status int,
	value any,
) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(
	w http.ResponseWriter,
	status int,
	message string,
) {
	writeJSON(
		w,
		status,
		map[string]string{
			"error": message,
		},
	)
}
