package auth

import (
	"backend/internal/config"
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"

	"backend/internal/user"
)

const (
	maxFailedLoginAttempts = 5
	lockDuration           = 15 * time.Minute
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountLocked      = errors.New("account locked")
	ErrEmailNotVerified   = errors.New("email not verified")
	ErrAccountDisabled    = errors.New("account disabled")
)

type LoginRequest struct {
	Email    string
	Password string

	IPAddress net.IP
	UserAgent string
}

type LoginResult struct {
	User               *user.User
	AccessToken        string
	RefreshToken       string
	RefreshTokenExpiry time.Time
}

type LoginService struct {
	users    *user.Repository
	tokens   *TokenService
	sessions *SessionService
	config   config.SecurityConfig
}

func NewLoginService(
	users *user.Repository,
	tokens *TokenService,
	sessions *SessionService,
	securityConfig config.SecurityConfig,
) *LoginService {
	return &LoginService{
		users:    users,
		tokens:   tokens,
		sessions: sessions,
		config:   securityConfig,
	}
}

func (s *LoginService) Login(
	ctx context.Context,
	req LoginRequest,
) (*LoginResult, error) {
	account, err := s.users.FindByEmail(
		ctx,
		req.Email,
	)

	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}

		return nil, fmt.Errorf(
			"find login user: %w",
			err,
		)
	}

	if account.LockedUntil != nil &&
		account.LockedUntil.After(time.Now()) {
		return nil, ErrAccountLocked
	}

	switch account.AccountStatus {
	case "disabled":
		return nil, ErrAccountDisabled

	case "pending":
		return nil, ErrEmailNotVerified
	}

	valid, err := s.users.VerifyPassword(
		ctx,
		account.ID,
		req.Password,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"verify password: %w",
			err,
		)
	}

	if !valid {
		if err := s.recordFailedLogin(
			ctx,
			account.ID,
			req.IPAddress,
			req.UserAgent,
		); err != nil {
			return nil, err
		}

		return nil, ErrInvalidCredentials
	}

	if err := s.recordSuccessfulLogin(
		ctx,
		account.ID,
		req.IPAddress,
	); err != nil {
		return nil, err
	}

	accessToken, err := s.tokens.CreateAccessToken(
		account.ID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create access token: %w",
			err,
		)
	}

	session, err := s.sessions.Create(
		ctx,
		account.ID,
		req.IPAddress,
		req.UserAgent,
		s.config.RefreshTokenTTL,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create session: %w",
			err,
		)
	}

	return &LoginResult{
		User:               account,
		AccessToken:        accessToken,
		RefreshToken:       session.RefreshToken,
		RefreshTokenExpiry: session.ExpiresAt,
	}, nil
}

func (s *LoginService) recordFailedLogin(
	ctx context.Context,
	userID uuid.UUID,
	ip net.IP,
	userAgent string,
) error {
	return s.users.RecordFailedLogin(
		ctx,
		userID,
		ip,
		userAgent,
	)
}

func (s *LoginService) recordSuccessfulLogin(
	ctx context.Context,
	userID uuid.UUID,
	ip net.IP,
) error {
	return s.users.RecordSuccessfulLogin(
		ctx,
		userID,
		ip,
	)
}
