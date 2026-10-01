package auth

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	tokenutil "backend/internal/token"
	"backend/internal/user"
)

var ErrInvalidResendRequest = errors.New("invalid resend request")

type ResendVerificationService struct {
	db *pgxpool.Pool
}

func NewResendVerificationService(
	db *pgxpool.Pool,
) *ResendVerificationService {
	return &ResendVerificationService{
		db: db,
	}
}

type ResendVerificationResult struct {
	UserID uuid.UUID
	Email  string
	Token  string
}

func (s *ResendVerificationService) Resend(
	ctx context.Context,
	email string,
	ip net.IP,
	userAgent string,
) (*ResendVerificationResult, error) {
	email = user.NormalizeEmail(email)

	if email == "" {
		return nil, ErrInvalidResendRequest
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"begin resend verification: %w",
			err,
		)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var (
		userID        uuid.UUID
		userEmail     string
		emailVerified bool
		accountStatus string
	)

	const findUser = `
		SELECT
			id,
			email,
			email_verified,
			account_status
		FROM users
		WHERE email = $1
		FOR UPDATE
	`

	err = tx.QueryRow(
		ctx,
		findUser,
		email,
	).Scan(
		&userID,
		&userEmail,
		&emailVerified,
		&accountStatus,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		/*
			Do not reveal whether the email exists.
			The handler will return the same generic response.
		*/
		return nil, ErrInvalidResendRequest
	}

	if err != nil {
		return nil, fmt.Errorf(
			"find user for verification resend: %w",
			err,
		)
	}

	/*
		Already verified accounts don't need another token.
	*/
	if emailVerified || accountStatus == "active" {
		return nil, ErrInvalidResendRequest
	}

	/*
		Invalidate every previous unused verification token.

		We don't delete them because keeping the records gives us
		a useful audit trail and prevents token reuse.
	*/
	const invalidateTokens = `
		UPDATE email_verifications
		SET used_at = now()
		WHERE user_id = $1
		  AND used_at IS NULL
	`

	if _, err := tx.Exec(
		ctx,
		invalidateTokens,
		userID,
	); err != nil {
		return nil, fmt.Errorf(
			"invalidate previous verification tokens: %w",
			err,
		)
	}

	rawToken, err := tokenutil.Generate(32)
	if err != nil {
		return nil, fmt.Errorf(
			"generate verification token: %w",
			err,
		)
	}

	tokenHash := tokenutil.Hash(rawToken)

	const createToken = `
		INSERT INTO email_verifications (
			user_id,
			token_hash,
			expires_at
		)
		VALUES (
			$1,
			$2,
			now() + interval '24 hours'
		)
	`

	if _, err := tx.Exec(
		ctx,
		createToken,
		userID,
		tokenHash,
	); err != nil {
		return nil, fmt.Errorf(
			"create verification token: %w",
			err,
		)
	}

	const securityEvent = `
		INSERT INTO security_events (
			user_id,
			event_type,
			ip_address,
			user_agent,
			metadata
		)
		VALUES (
			$1,
			'EMAIL_VERIFICATION_RESENT',
			$2,
			$3,
			'{}'
		)
	`

	if _, err := tx.Exec(
		ctx,
		securityEvent,
		userID,
		ip,
		userAgent,
	); err != nil {
		return nil, fmt.Errorf(
			"record verification resend event: %w",
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf(
			"commit verification resend: %w",
			err,
		)
	}

	return &ResendVerificationResult{
		UserID: userID,
		Email:  userEmail,
		Token:  rawToken,
	}, nil
}
