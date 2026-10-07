package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"backend/internal/email"
	"backend/internal/password"
	"backend/internal/token"
	"backend/internal/user"

	"github.com/google/uuid"
)

var (
	ErrInvalidPasswordResetToken = errors.New(
		"invalid or expired password reset token",
	)

	ErrPasswordResetRequestFailed = errors.New(
		"If the account exists, you will receive a password reset email.",
	)
)

type PasswordResetRequest struct {
	Email     string
	Locale    string
	IPAddress net.IP
	UserAgent string
}

type PasswordResetConfirmRequest struct {
	Token     string
	Password  string
	IPAddress net.IP
	UserAgent string
}

type PasswordResetService struct {
	users       *user.Repository
	resets      *PasswordResetRepository
	emailSender email.Sender
}

func NewPasswordResetService(
	users *user.Repository,
	resets *PasswordResetRepository,
	emailSender email.Sender,
) *PasswordResetService {
	return &PasswordResetService{
		users:       users,
		resets:      resets,
		emailSender: emailSender,
	}
}

func (s *PasswordResetService) Request(
	ctx context.Context,
	req PasswordResetRequest,
) error {
	emailAddress := user.NormalizeEmail(req.Email)

	if emailAddress == "" {
		return nil
	}

	// Always generate the token before looking up the account.
	// This makes the request path less distinguishable by timing.
	resetToken, err := token.Generate(32)
	if err != nil {
		return fmt.Errorf(
			"generate password reset token: %w",
			err,
		)
	}

	tokenHash := token.Hash(resetToken)

	account, err := s.users.FindByEmail(
		ctx,
		emailAddress,
	)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			// Deliberately return success.
			// Never reveal whether the email exists.
			return nil
		}

		return fmt.Errorf(
			"find password reset account: %w",
			err,
		)
	}

	// Do not allow password reset for accounts that are not
	// fully activated.
	if !account.EmailVerified ||
		account.AccountStatus != "active" {
		return nil
	}

	if err := s.resets.Create(
		ctx,
		account.ID,
		tokenHash,
	); err != nil {
		return fmt.Errorf(
			"create password reset: %w",
			err,
		)
	}

	if err := s.emailSender.SendPasswordResetEmail(
		ctx,
		email.PasswordResetEmail{
			To:       account.Email,
			FullName: account.FullName,
			Token:    resetToken,
			Locale:   req.Locale,
		},
	); err != nil {
		return fmt.Errorf(
			"send password reset email: %w",
			err,
		)
	}

	return nil
}

func (s *PasswordResetService) Confirm(
	ctx context.Context,
	req PasswordResetConfirmRequest,
) error {
	resetToken := strings.TrimSpace(req.Token)

	if resetToken == "" {
		return ErrInvalidPasswordResetToken
	}

	if err := validatePassword(
		req.Password,
	); err != nil {
		return err
	}

	passwordHash, err := password.Hash(
		req.Password,
	)
	if err != nil {
		return fmt.Errorf(
			"hash password: %w",
			err,
		)
	}

	_, err = s.resets.Confirm(
		ctx,
		token.Hash(resetToken),
		passwordHash,
		ipString(req.IPAddress),
		req.UserAgent,
	)
	if err != nil {
		return err
	}

	return nil
}

func ipString(ip net.IP) string {
	if ip == nil {
		return ""
	}

	return ip.String()
}

var _ = uuid.Nil
