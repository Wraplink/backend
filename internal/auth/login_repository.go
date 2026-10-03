package auth

import (
	"backend/internal/user"
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type LoginUser struct {
	ID                  uuid.UUID
	Email               string
	FullName            string
	PasswordHash        string
	EmailVerified       bool
	AccountStatus       string
	FailedLoginAttempts int
	LockedUntil         *time.Time
}

type LoginRepository struct {
	db *pgxpool.Pool
}

func NewLoginRepository(db *pgxpool.Pool) *LoginRepository {
	return &LoginRepository{
		db: db,
	}
}

func (r *LoginRepository) FindForLogin(
	ctx context.Context,
	email string,
) (*LoginUser, error) {
	email = user.NormalizeEmail(email)

	const query = `
		SELECT
			u.id,
			u.email,
			u.full_name,
			uc.password_hash,
			u.email_verified,
			u.account_status,
			u.failed_login_attempts,
			u.locked_until
		FROM users u
		INNER JOIN user_credentials uc
			ON uc.user_id = u.id
		WHERE u.email = $1
	`

	var result LoginUser

	err := r.db.QueryRow(
		ctx,
		query,
		email,
	).Scan(
		&result.ID,
		&result.Email,
		&result.FullName,
		&result.PasswordHash,
		&result.EmailVerified,
		&result.AccountStatus,
		&result.FailedLoginAttempts,
		&result.LockedUntil,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}

	if err != nil {
		return nil, fmt.Errorf(
			"find user for login: %w",
			err,
		)
	}

	return &result, nil
}

func (r *LoginRepository) RecordFailedLogin(
	ctx context.Context,
	userID uuid.UUID,
	ip net.IP,
) error {
	const query = `
		UPDATE users
		SET
			failed_login_attempts = failed_login_attempts + 1,
			locked_until =
				CASE
					WHEN failed_login_attempts + 1 >= 5
					THEN now() + interval '15 minutes'
					ELSE locked_until
				END,
			updated_at = now()
		WHERE id = $1
	`

	if _, err := r.db.Exec(
		ctx,
		query,
		userID,
	); err != nil {
		return fmt.Errorf(
			"record failed login: %w",
			err,
		)
	}

	const eventQuery = `
		INSERT INTO security_events (
			user_id,
			event_type,
			ip_address
		)
		VALUES (
			$1,
			'LOGIN_FAILED',
			$2
		)
	`

	if _, err := r.db.Exec(
		ctx,
		eventQuery,
		userID,
		ip,
	); err != nil {
		return fmt.Errorf(
			"record failed login event: %w",
			err,
		)
	}

	return nil
}

func (r *LoginRepository) RecordSuccessfulLogin(
	ctx context.Context,
	userID uuid.UUID,
	ip net.IP,
) error {
	const query = `
		UPDATE users
		SET
			failed_login_attempts = 0,
			locked_until = NULL,
			last_login_at = now(),
			last_login_ip = $2,
			updated_at = now()
		WHERE id = $1
	`

	if _, err := r.db.Exec(
		ctx,
		query,
		userID,
		ip,
	); err != nil {
		return fmt.Errorf(
			"record successful login: %w",
			err,
		)
	}

	const eventQuery = `
		INSERT INTO security_events (
			user_id,
			event_type,
			ip_address
		)
		VALUES (
			$1,
			'LOGIN_SUCCESS',
			$2
		)
	`

	if _, err := r.db.Exec(
		ctx,
		eventQuery,
		userID,
		ip,
	); err != nil {
		return fmt.Errorf(
			"record successful login event: %w",
			err,
		)
	}

	return nil
}

func isLocked(user *LoginUser) bool {
	if user.LockedUntil == nil {
		return false
	}

	return time.Now().Before(*user.LockedUntil)
}
