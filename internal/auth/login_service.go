package auth

import (
	"backend/internal/config"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/google/uuid"

	"backend/internal/password"
)

type LoginRequest struct {
	Email     string
	Password  string
	IPAddress net.IP
	UserAgent string
}

type LoginResult struct {
	UserID             uuid.UUID
	Email              string
	FullName           string
	AccessToken        string
	RefreshToken       string
	RefreshTokenExpiry time.Time
}

type LoginService struct {
	repository   *LoginRepository
	tokenService *TokenService
	sessions     *SessionService
	config       config.SecurityConfig
}

func NewLoginService(
	repository *LoginRepository,
	tokenService *TokenService,
	sessions *SessionService,
	config config.SecurityConfig,
) *LoginService {
	return &LoginService{
		repository:   repository,
		tokenService: tokenService,
		sessions:     sessions,
		config:       config,
	}
}

func (s *LoginService) Login(
	ctx context.Context,
	req LoginRequest,
) (*LoginResult, error) {
	email := strings.TrimSpace(req.Email)

	if email == "" || req.Password == "" {
		return nil, ErrInvalidCredentials
	}

	account, err := s.repository.FindForLogin(
		ctx,
		email,
	)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return nil, ErrInvalidCredentials
		}

		return nil, fmt.Errorf(
			"load login account: %w",
			err,
		)
	}

	// Never reveal account state to the client.
	if isLocked(account) {
		return nil, ErrInvalidCredentials
	}

	if account.AccountStatus != "active" {
		return nil, ErrInvalidCredentials
	}

	if !account.EmailVerified {
		return nil, ErrInvalidCredentials
	}

	if !password.Verify(
		req.Password,
		account.PasswordHash,
	) {
		if err := s.repository.RecordFailedLogin(
			ctx,
			account.ID,
			req.IPAddress,
		); err != nil {
			return nil, fmt.Errorf(
				"record failed login: %w",
				err,
			)
		}

		return nil, ErrInvalidCredentials
	}

	if err := s.repository.RecordSuccessfulLogin(
		ctx,
		account.ID,
		req.IPAddress,
	); err != nil {
		return nil, fmt.Errorf(
			"record successful login: %w",
			err,
		)
	}

	accessToken, err := s.tokenService.CreateAccessToken(
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
		UserID:             account.ID,
		Email:              account.Email,
		FullName:           account.FullName,
		AccessToken:        accessToken,
		RefreshToken:       session.RefreshToken,
		RefreshTokenExpiry: session.ExpiresAt,
	}, nil
}
