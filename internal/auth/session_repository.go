package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	tokenutil "backend/internal/token"
)

type SessionRepository struct {
	db *pgxpool.Pool
}

func NewSessionRepository(db *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{
		db: db,
	}
}

func (r *SessionRepository) Create(
	ctx context.Context,
	userID uuid.UUID,
	refreshToken string,
	ipAddress net.IP,
	userAgent string,
	expiresAt time.Time,
) (uuid.UUID, error) {
	const query = `
		INSERT INTO user_sessions (
			user_id,
			refresh_token_hash,
			ip_address,
			user_agent,
			expires_at
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`

	var sessionID uuid.UUID

	err := r.db.QueryRow(
		ctx,
		query,
		userID,
		tokenutil.Hash(refreshToken),
		ipAddress,
		userAgent,
		expiresAt,
	).Scan(&sessionID)

	if err != nil {
		return uuid.Nil, fmt.Errorf(
			"create user session: %w",
			err,
		)
	}

	return sessionID, nil
}

func (r *SessionRepository) RevokeByRefreshToken(
	ctx context.Context,
	refreshToken string,
) (uuid.UUID, uuid.UUID, error) {
	const query = `
        UPDATE user_sessions
        SET revoked_at = now()
        WHERE refresh_token_hash = $1
          AND revoked_at IS NULL
        RETURNING id, user_id
    `

	var sessionID uuid.UUID
	var userID uuid.UUID

	err := r.db.QueryRow(
		ctx,
		query,
		tokenutil.Hash(refreshToken),
	).Scan(
		&sessionID,
		&userID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, uuid.Nil, nil
		}

		return uuid.Nil, uuid.Nil,
			fmt.Errorf("revoke user session: %w", err)
	}

	return sessionID, userID, nil
}
