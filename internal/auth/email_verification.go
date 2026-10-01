package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	tokenutil "backend/internal/token"
)

var (
	ErrInvalidVerificationToken = errors.New("invalid verification token")
	ErrVerificationExpired      = errors.New("verification token expired")
	ErrVerificationUsed         = errors.New("verification token already used")
	ErrEmailAlreadyVerified     = errors.New("email already verified")
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
func (s *EmailVerificationService) Verify(
	ctx context.Context,
	rawToken string,
) error {
	if rawToken == "" {
		return ErrInvalidVerificationToken
	}

	tokenHash := tokenutil.Hash(rawToken)

	tx, err := s.db.Begin(ctx)

	if err != nil {
		return fmt.Errorf("begin email verification: %w", err)
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

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidVerificationToken
	}

	if err != nil {
		return fmt.Errorf(
			"find email verification: %w",
			err,
		)
	}

	if usedAt != nil {
		return ErrVerificationUsed
	}

	if !expiresAt.After(time.Now().UTC()) {
		return ErrVerificationExpired
	}

	var alreadyVerified bool

	const checkUser = `
		SELECT email_verified
		FROM users
		WHERE id = $1
		FOR UPDATE
	`

	err = tx.QueryRow(
		ctx,
		checkUser,
		userID,
	).Scan(&alreadyVerified)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidVerificationToken
	}

	if err != nil {
		return fmt.Errorf(
			"check email verification status: %w",
			err,
		)
	}

	if alreadyVerified {
		return ErrEmailAlreadyVerified
	}

	const markUserVerified = `
		UPDATE users
		SET
			email_verified = true,
			account_status = 'active',
			updated_at = now()
		WHERE id = $1
	`

	if _, err := tx.Exec(
		ctx,
		markUserVerified,
		userID,
	); err != nil {
		return fmt.Errorf(
			"activate verified user: %w",
			err,
		)
	}

	const markTokenUsed = `
		UPDATE email_verifications
		SET used_at = now()
		WHERE id = $1
	`

	if _, err := tx.Exec(
		ctx,
		markTokenUsed,
		verificationID,
	); err != nil {
		return fmt.Errorf(
			"mark verification token used: %w",
			err,
		)
	}

	const securityEvent = `
		INSERT INTO security_events (
			user_id,
			event_type,
			metadata
		)
		VALUES (
			$1,
			'EMAIL_VERIFIED',
			'{}'
		)
	`

	if _, err := tx.Exec(
		ctx,
		securityEvent,
		userID,
	); err != nil {
		return fmt.Errorf(
			"record email verification event: %w",
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
