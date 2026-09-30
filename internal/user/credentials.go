package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"backend/internal/password"
)

func (r *Repository) GetPasswordHash(
	ctx context.Context,
	userID uuid.UUID,
) (string, error) {
	const query = `
		SELECT password_hash
		FROM user_credentials
		WHERE user_id = $1
	`

	var hash string

	err := r.db.QueryRow(
		ctx,
		query,
		userID,
	).Scan(&hash)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrUserNotFound
	}

	if err != nil {
		return "", fmt.Errorf(
			"get password hash: %w",
			err,
		)
	}

	return hash, nil
}

func (r *Repository) VerifyPassword(
	ctx context.Context,
	userID uuid.UUID,
	rawPassword string,
) (bool, error) {
	hash, err := r.GetPasswordHash(ctx, userID)

	if err != nil {
		return false, err
	}

	return password.Verify(rawPassword, hash), nil
}
