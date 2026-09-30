package user

import (
	"context"
	"fmt"
	"net"

	"github.com/google/uuid"
)

func (r *Repository) RecordFailedLogin(
	ctx context.Context,
	userID uuid.UUID,
	ip net.IP,
	userAgent string,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin failed login transaction: %w", err)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const updateUser = `
		UPDATE users
		SET
			failed_login_attempts = failed_login_attempts + 1,
			locked_until = CASE
				WHEN failed_login_attempts + 1 >= 5
				THEN now() + interval '15 minutes'
				ELSE locked_until
			END,
			updated_at = now()
		WHERE id = $1
	`

	if _, err := tx.Exec(
		ctx,
		updateUser,
		userID,
	); err != nil {
		return fmt.Errorf("record failed login: %w", err)
	}

	const event = `
		INSERT INTO security_events (
			user_id,
			event_type,
			ip_address,
			user_agent
		)
		VALUES ($1, 'LOGIN_FAILED', $2, $3)
	`

	if _, err := tx.Exec(
		ctx,
		event,
		userID,
		ip,
		userAgent,
	); err != nil {
		return fmt.Errorf(
			"record failed login event: %w",
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf(
			"commit failed login: %w",
			err,
		)
	}

	return nil
}

func (r *Repository) RecordSuccessfulLogin(
	ctx context.Context,
	userID uuid.UUID,
	ip net.IP,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf(
			"begin successful login transaction: %w",
			err,
		)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const updateUser = `
		UPDATE users
		SET
			failed_login_attempts = 0,
			locked_until = NULL,
			last_login_at = now(),
			last_login_ip = $2,
			updated_at = now()
		WHERE id = $1
	`

	if _, err := tx.Exec(
		ctx,
		updateUser,
		userID,
		ip,
	); err != nil {
		return fmt.Errorf(
			"record successful login: %w",
			err,
		)
	}

	const event = `
		INSERT INTO security_events (
			user_id,
			event_type,
			ip_address
		)
		VALUES ($1, 'LOGIN_SUCCESS', $2)
	`

	if _, err := tx.Exec(
		ctx,
		event,
		userID,
		ip,
	); err != nil {
		return fmt.Errorf(
			"record successful login event: %w",
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf(
			"commit successful login: %w",
			err,
		)
	}

	return nil
}
