package auth

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PasswordChangeRepository struct {
	db *pgxpool.Pool
}

func NewPasswordChangeRepository(db *pgxpool.Pool) *PasswordChangeRepository {
	if db == nil {
		panic("auth: nil database pool")
	}

	return &PasswordChangeRepository{db: db}
}

func (r *PasswordChangeRepository) FindPasswordHash(
	ctx context.Context,
	userID uuid.UUID,
) (string, error) {
	const query = `
		SELECT password_hash
		FROM user_credentials
		WHERE user_id = $1
	`

	var hash string
	err := r.db.QueryRow(ctx, query, userID).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("password credentials not found")
	}
	if err != nil {
		return "", fmt.Errorf("find password credentials: %w", err)
	}

	return hash, nil
}

func (r *PasswordChangeRepository) Change(
	ctx context.Context,
	userID uuid.UUID,
	newPasswordHash string,
	ipAddress net.IP,
	userAgent string,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin password change transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const updatePassword = `
		UPDATE user_credentials
		SET password_hash = $2,
		    password_changed_at = now()
		WHERE user_id = $1
	`

	result, err := tx.Exec(ctx, updatePassword, userID, newPasswordHash)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if result.RowsAffected() != 1 {
		return errors.New("password credentials not found")
	}

	const revokeSessions = `
		UPDATE user_sessions
		SET revoked_at = now()
		WHERE user_id = $1
		  AND revoked_at IS NULL
	`
	if _, err := tx.Exec(ctx, revokeSessions, userID); err != nil {
		return fmt.Errorf("revoke sessions after password change: %w", err)
	}

	const resetLockout = `
		UPDATE users
		SET failed_login_attempts = 0,
		    locked_until = NULL,
		    updated_at = now()
		WHERE id = $1
	`
	if _, err := tx.Exec(ctx, resetLockout, userID); err != nil {
		return fmt.Errorf("reset login lockout: %w", err)
	}

	const recordEvent = `
		INSERT INTO security_events (
			user_id, event_type, ip_address, user_agent
		)
		VALUES ($1, 'PASSWORD_CHANGED', $2, $3)
	`
	if _, err := tx.Exec(ctx, recordEvent, userID, ipAddress, userAgent); err != nil {
		return fmt.Errorf("record password change event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit password change: %w", err)
	}

	return nil
}
