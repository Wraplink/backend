package user

import (
	"encoding/json"
	"net/http"
	"time"
)

func writeMeResponse(
	w http.ResponseWriter,
	user *User,
) {
	response := MeResponse{
		ID:            user.ID.String(),
		Email:         user.Email,
		FullName:      user.FullName,
		EmailVerified: user.EmailVerified,
		AccountStatus: user.AccountStatus,
		CreatedAt:     user.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     user.UpdatedAt.UTC().Format(time.RFC3339),
	}

	if user.LastLoginAt != nil {
		value := user.LastLoginAt.UTC().Format(time.RFC3339)
		response.LastLoginAt = &value
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	_ = json.NewEncoder(w).Encode(response)
}
