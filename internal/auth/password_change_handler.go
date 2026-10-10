package auth

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"backend/internal/requestcontext"
	"backend/internal/security"
)

type PasswordChangeHandler struct {
	service      *PasswordChangeService
	trustedProxy *security.TrustedProxy
}

func NewPasswordChangeHandler(
	service *PasswordChangeService,
	trustedProxy *security.TrustedProxy,
) *PasswordChangeHandler {
	if service == nil || trustedProxy == nil {
		panic("auth: nil password change handler dependency")
	}

	return &PasswordChangeHandler{
		service:      service,
		trustedProxy: trustedProxy,
	}
}

type changePasswordBody struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (h *PasswordChangeHandler) Change(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := requestcontext.UserID(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var body changePasswordBody
	if err := decoder.Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ip := h.trustedProxy.ClientIP(r)

	err := h.service.Change(r.Context(), PasswordChangeRequest{
		UserID:          userID,
		CurrentPassword: body.CurrentPassword,
		NewPassword:     body.NewPassword,
		IPAddress:       ip,
		UserAgent:       r.UserAgent(),
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCurrentPassword):
			writeJSONError(w, http.StatusUnauthorized, "current password is incorrect")
		case errors.Is(err, ErrPasswordUnchanged):
			writeJSONError(w, http.StatusBadRequest, "new password must differ from current password")
		case errors.Is(err, ErrWeakPassword):
			writeJSONError(w, http.StatusBadRequest, "new password does not meet password requirements")
		default:
			writeJSONError(w, http.StatusInternalServerError, "could not change password")
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"message":"password changed successfully"}`))
}
