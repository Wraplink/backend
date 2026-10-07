package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PasswordResetRepository struct {
	db *pgxpool.Pool
}

func NewPasswordResetRepository(
	db *pgxpool.Pool,
) *PasswordResetRepository {
	return &PasswordResetRepository{
		db: db,
	}
}

func (r *PasswordResetRepository) Create(
	ctx context.Context,
	userID uuid.UUID,
	tokenHash string,
) error {
	const query = `
		INSERT INTO password_resets (
			user_id,
			token_hash,
			expires_at
		)
		VALUES (
			$1,
			$2,
			now() + interval '30 minutes'
		)
	`

	_, err := r.db.Exec(
		ctx,
		query,
		userID,
		tokenHash,
	)
	if err != nil {
		return fmt.Errorf(
			"create password reset token: %w",
			err,
		)
	}

	return nil
}

// Confirm consumes the reset token and changes the password
// atomically.
//
// The following operations happen in ONE transaction:
//
//  1. Validate and consume reset token.
//  2. Update password.
//  3. Revoke all refresh sessions.
//  4. Record security event.
//
// If any operation fails, everything is rolled back.
func (r *PasswordResetRepository) Confirm(
	ctx context.Context,
	tokenHash string,
	passwordHash string,
	ipAddress string,
	userAgent string,
) (uuid.UUID, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf(
			"begin password reset transaction: %w",
			err,
		)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var userID uuid.UUID

	const consumeToken = `
		UPDATE password_resets
		SET used_at = now()
		WHERE token_hash = $1
		  AND used_at IS NULL
		  AND expires_at > now()
		RETURNING user_id
	`

	err = tx.QueryRow(
		ctx,
		consumeToken,
		tokenHash,
	).Scan(&userID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrInvalidPasswordResetToken
		}

		return uuid.Nil, fmt.Errorf(
			"consume password reset token: %w",
			err,
		)
	}

	const updatePassword = `
		UPDATE user_credentials
		SET
			password_hash = $2,
			password_changed_at = now()
		WHERE user_id = $1
	`

	result, err := tx.Exec(
		ctx,
		updatePassword,
		userID,
		passwordHash,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf(
			"update password: %w",
			err,
		)
	}

	if result.RowsAffected() != 1 {
		return uuid.Nil, fmt.Errorf(
			"user credentials not found for user %s",
			userID,
		)
	}

	const revokeSessions = `
		UPDATE user_sessions
		SET revoked_at = now()
		WHERE user_id = $1
		  AND revoked_at IS NULL
	`

	_, err = tx.Exec(
		ctx,
		revokeSessions,
		userID,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf(
			"revoke user sessions: %w",
			err,
		)
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
			'PASSWORD_RESET',
			$2,
			$3
		)
	`

	_, err = tx.Exec(
		ctx,
		securityEvent,
		userID,
		ipAddress,
		userAgent,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf(
			"record password reset security event: %w",
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf(
			"commit password reset: %w",
			err,
		)
	}

	return userID, nil
}
