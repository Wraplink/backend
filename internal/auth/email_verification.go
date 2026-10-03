package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"backend/internal/token"
)

var (
	ErrInvalidVerificationToken = errors.New("invalid verification token")
)

type EmailVerificationService struct {
	db *pgxpool.Pool
}

func NewEmailVerificationService(
	db *pgxpool.Pool,
) *EmailVerificationService {
	return &EmailVerificationService{
		db: db,
	}
}

type VerifyEmailRequest struct {
	Token     string
	IPAddress net.IP
	UserAgent string
}

func (s *EmailVerificationService) Verify(
	ctx context.Context,
	req VerifyEmailRequest,
) error {
	rawToken := strings.TrimSpace(req.Token)

	if rawToken == "" {
		return ErrInvalidVerificationToken
	}

	tokenHash := token.Hash(rawToken)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf(
			"begin email verification transaction: %w",
			err,
		)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var (
		verificationID uuid.UUID
		userID         uuid.UUID
		expiresAt      time.Time
		usedAt         *time.Time
	)

	const findVerification = `
		SELECT
			id,
			user_id,
			expires_at,
			used_at
		FROM email_verifications
		WHERE token_hash = $1
		FOR UPDATE
	`

	err = tx.QueryRow(
		ctx,
		findVerification,
		tokenHash,
	).Scan(
		&verificationID,
		&userID,
		&expiresAt,
		&usedAt,
	)

	if err != nil {
		return ErrInvalidVerificationToken
	}

	if usedAt != nil {
		return ErrInvalidVerificationToken
	}

	if !time.Now().Before(expiresAt) {
		return ErrInvalidVerificationToken
	}

	const markUsed = `
		UPDATE email_verifications
		SET used_at = now()
		WHERE id = $1
	`

	if _, err := tx.Exec(
		ctx,
		markUsed,
		verificationID,
	); err != nil {
		return fmt.Errorf(
			"mark email verification used: %w",
			err,
		)
	}

	const activateUser = `
		UPDATE users
		SET
			email_verified = TRUE,
			account_status = 'active',
			updated_at = now()
		WHERE id = $1
	`

	result, err := tx.Exec(
		ctx,
		activateUser,
		userID,
	)

	if err != nil {
		return fmt.Errorf(
			"activate verified user: %w",
			err,
		)
	}

	if result.RowsAffected() != 1 {
		return ErrInvalidVerificationToken
	}

	const securityEvent = `
		INSERT INTO security_events (
			user_id,
			event_type,
			ip_address,
			user_agent
		)
		VALUES (
			$1,
			'EMAIL_VERIFIED',
			$2,
			$3
		)
	`

	if _, err := tx.Exec(
		ctx,
		securityEvent,
		userID,
		req.IPAddress,
		req.UserAgent,
	); err != nil {
		return fmt.Errorf(
			"create email verification security event: %w",
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf(
			"commit email verification: %w",
			err,
		)
	}

	return nil
}
