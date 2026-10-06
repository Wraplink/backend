package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

var ErrInvalidRefreshToken = errors.New("invalid refresh token")

type SessionRevoker interface {
	RevokeByRefreshToken(
		ctx context.Context,
		refreshToken string,
	) (uuid.UUID, uuid.UUID, error)
}

type LogoutService struct {
	sessionRepository SessionRevoker
}

func NewLogoutService(
	sessionRepository SessionRevoker,
) *LogoutService {
	return &LogoutService{
		sessionRepository: sessionRepository,
	}
}

type LogoutResult struct {
	SessionID uuid.UUID
	UserID    uuid.UUID
	Revoked   bool
}

func (s *LogoutService) Logout(
	ctx context.Context,
	refreshToken string,
) (*LogoutResult, error) {
	refreshToken = strings.TrimSpace(refreshToken)

	// Logout is intentionally idempotent.
	if refreshToken == "" {
		return &LogoutResult{}, nil
	}

	sessionID, userID, err :=
		s.sessionRepository.RevokeByRefreshToken(
			ctx,
			refreshToken,
		)

	if err != nil {
		return nil, err
	}

	if sessionID == uuid.Nil {
		return &LogoutResult{
			Revoked: false,
		}, nil
	}

	return &LogoutResult{
		SessionID: sessionID,
		UserID:    userID,
		Revoked:   true,
	}, nil
}
