package auth

import (
	"context"
	"fmt"
	"net"
	"time"

	"backend/internal/config"
)

type RefreshService struct {
	sessions     *SessionService
	tokenService *TokenService
	config       config.SecurityConfig
}

func NewRefreshService(
	sessions *SessionService,
	tokens *TokenService,
	securityConfig config.SecurityConfig,
) *RefreshService {
	return &RefreshService{
		sessions:     sessions,
		tokenService: tokens,
		config:       securityConfig,
	}
}

type RefreshResult struct {
	AccessToken        string
	RefreshToken       string
	RefreshTokenExpiry time.Time
	UserID             string
}

func (s *RefreshService) Refresh(
	ctx context.Context,
	refreshToken string,
	ip net.IP,
	userAgent string,
) (*RefreshResult, error) {
	newRefreshToken, refreshUntil, err :=
		s.tokenService.CreateRefreshToken()

	if err != nil {
		return nil, fmt.Errorf(
			"create refresh token: %w",
			err,
		)
	}

	session, err := s.sessions.Rotate(
		ctx,
		refreshToken,
		ip,
		userAgent,
		s.config.RefreshTokenTTL,
		newRefreshToken,
	)
	if err != nil {
		return nil, err
	}

	accessToken, err := s.tokenService.CreateAccessToken(
		session.UserID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create refreshed access token: %w",
			err,
		)
	}

	return &RefreshResult{
		AccessToken:        accessToken,
		RefreshToken:       newRefreshToken,
		RefreshTokenExpiry: refreshUntil,
		UserID:             session.UserID.String(),
	}, nil
}
