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

type ManagedSession struct {
	ID        uuid.UUID `json:"id"`
	IPAddress *string   `json:"ipAddress"`
	UserAgent string    `json:"userAgent"`
	ExpiresAt time.Time `json:"expiresAt"`
}

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

func (r *SessionRepository) ListActive(
	ctx context.Context,
	userID uuid.UUID,
) ([]ManagedSession, error) {
	const query = `
		SELECT
			id,
			host(ip_address),
			COALESCE(user_agent, ''),
			expires_at
		FROM user_sessions
		WHERE user_id = $1
		  AND revoked_at IS NULL
		  AND expires_at > now()
		ORDER BY expires_at DESC
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list active sessions: %w", err)
	}
	defer rows.Close()

	sessions := make([]ManagedSession, 0)

	for rows.Next() {
		var session ManagedSession

		if err := rows.Scan(
			&session.ID,
			&session.IPAddress,
			&session.UserAgent,
			&session.ExpiresAt,
		); err != nil {
			return nil, fmt.Errorf("scan active session: %w", err)
		}

		sessions = append(sessions, session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active sessions: %w", err)
	}

	return sessions, nil
}

func (r *SessionRepository) RevokeByID(
	ctx context.Context,
	userID uuid.UUID,
	sessionID uuid.UUID,
) (bool, error) {
	const query = `
		UPDATE user_sessions
		SET revoked_at = now()
		WHERE id = $1
		  AND user_id = $2
		  AND revoked_at IS NULL
		  AND expires_at > now()
	`

	result, err := r.db.Exec(ctx, query, sessionID, userID)
	if err != nil {
		return false, fmt.Errorf("revoke session by ID: %w", err)
	}

	return result.RowsAffected() == 1, nil
}

func (r *SessionRepository) RevokeOthers(
	ctx context.Context,
	userID uuid.UUID,
	currentSessionID uuid.UUID,
) (int64, error) {
	const query = `
		UPDATE user_sessions
		SET
			revoked_at = now(),
			revoked_reason = 'revoked_others'
		WHERE user_id = $1
		  AND id <> $2
		  AND revoked_at IS NULL
		  AND expires_at > now()
	`

	result, err := r.db.Exec(
		ctx,
		query,
		userID,
		currentSessionID,
	)
	if err != nil {
		return 0, fmt.Errorf("revoke other sessions: %w", err)
	}

	return result.RowsAffected(), nil
}
