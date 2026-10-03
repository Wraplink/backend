package auth

import (
	"context"
	"fmt"
	"net"
	"time"

	"backend/internal/config"
)

type RefreshService struct {
	sessions *SessionService
	tokens   *TokenService
	config   config.SecurityConfig
}

func NewRefreshService(
	sessions *SessionService,
	tokens *TokenService,
	securityConfig config.SecurityConfig,
) *RefreshService {
	return &RefreshService{
		sessions: sessions,
		tokens:   tokens,
		config:   securityConfig,
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
	session, err := s.sessions.Rotate(
		ctx,
		refreshToken,
		ip,
		userAgent,
		s.config.RefreshTokenTTL,
	)
	if err != nil {
		return nil, err
	}

	accessToken, err := s.tokens.CreateAccessToken(
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
		RefreshToken:       session.RefreshToken,
		RefreshTokenExpiry: session.ExpiresAt,
		UserID:             session.UserID.String(),
	}, nil
}
