package user

import (
	"backend/internal/requestcontext"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type MeHandler struct {
	service *MeService
}

func NewMeHandler(
	service *MeService,
) *MeHandler {
	return &MeHandler{
		service: service,
	}
}

type UpdateMeRequest struct {
	FullName string `json:"full_name"`
}

type MeResponse struct {
	ID            string  `json:"id"`
	Email         string  `json:"email"`
	FullName      string  `json:"full_name"`
	EmailVerified bool    `json:"email_verified"`
	AccountStatus string  `json:"account_status"`
	LastLoginAt   *string `json:"last_login_at,omitempty"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

func (h *MeHandler) Get(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := requestcontext.UserID(
		r.Context(),
	)
	if !ok {
		http.Error(
			w,
			"authentication required",
			http.StatusUnauthorized,
		)
		return
	}

	user, err := h.service.Get(
		r.Context(),
		userID,
	)

	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			http.Error(
				w,
				"user not found",
				http.StatusNotFound,
			)
			return
		}

		http.Error(
			w,
			"failed to load user",
			http.StatusInternalServerError,
		)
		return
	}

	writeMeResponse(w, user)
}

func (h *MeHandler) Update(
	w http.ResponseWriter,
	r *http.Request,
) {
	userID, ok := requestcontext.UserID(r.Context())

	if !ok {
		http.Error(
			w,
			"authentication required",
			http.StatusUnauthorized,
		)
		return
	}

	var request UpdateMeRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(
			w,
			"invalid request body",
			http.StatusBadRequest,
		)
		return
	}

	request.FullName = strings.TrimSpace(
		request.FullName,
	)

	if request.FullName == "" {
		http.Error(
			w,
			"full_name is required",
			http.StatusBadRequest,
		)
		return
	}

	if len([]rune(request.FullName)) > 150 {
		http.Error(
			w,
			"full_name is too long",
			http.StatusBadRequest,
		)
		return
	}

	user, err := h.service.Update(
		r.Context(),
		userID,
		request.FullName,
	)

	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			http.Error(
				w,
				"user not found",
				http.StatusNotFound,
			)
			return
		}

		http.Error(
			w,
			"failed to update user",
			http.StatusInternalServerError,
		)
		return
	}

	writeMeResponse(w, user)
}
