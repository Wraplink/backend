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

var (
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrRefreshTokenReplay  = errors.New("refresh token replay detected")
)

type SessionService struct {
	db *pgxpool.Pool
}

func NewSessionService(db *pgxpool.Pool) *SessionService {
	return &SessionService{
		db: db,
	}
}

type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	FamilyID  uuid.UUID
	ExpiresAt time.Time
}

func (s *SessionService) Create(
	ctx context.Context,
	userID uuid.UUID,
	ip net.IP,
	userAgent string,
	ttl time.Duration,
	refreshToken string,
) (*Session, error) {

	tokenHash := tokenutil.Hash(refreshToken)

	expiresAt := time.Now().UTC().Add(ttl)

	familyID := uuid.New()

	var sessionID uuid.UUID

	const query = `
		INSERT INTO user_sessions (
			user_id,
			refresh_token_hash,
			ip_address,
			user_agent,
			expires_at,
			family_id
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`

	err := s.db.QueryRow(
		ctx,
		query,
		userID,
		tokenHash,
		ip,
		userAgent,
		expiresAt,
		familyID,
	).Scan(&sessionID)

	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return &Session{
		ID:        sessionID,
		UserID:    userID,
		FamilyID:  familyID,
		ExpiresAt: expiresAt,
	}, nil
}

func (s *SessionService) Rotate(
	ctx context.Context,
	refreshToken string,
	ip net.IP,
	userAgent string,
	ttl time.Duration,
	newRefreshToken string,
) (*Session, error) {
	if refreshToken == "" {
		return nil, ErrInvalidRefreshToken
	}

	tokenHash := tokenutil.Hash(refreshToken)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin refresh rotation: %w", err)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var (
		oldSessionID uuid.UUID
		userID       uuid.UUID
		familyID     uuid.UUID
		expiresAt    time.Time
		revokedAt    *time.Time
	)

	const findSession = `
		SELECT
			id,
			user_id,
			family_id,
			expires_at,
			revoked_at
		FROM user_sessions
		WHERE refresh_token_hash = $1
		FOR UPDATE
	`

	err = tx.QueryRow(
		ctx,
		findSession,
		tokenHash,
	).Scan(
		&oldSessionID,
		&userID,
		&familyID,
		&expiresAt,
		&revokedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidRefreshToken
	}

	if err != nil {
		return nil, fmt.Errorf("find refresh session: %w", err)
	}

	/*
		An already revoked refresh token means somebody is
		attempting to reuse an old token.

		This is especially important after rotation:

		    token A
		      |
		      +----> token B
		      |
		    revoked

		If token A appears again, it is a replay.
	*/
	if revokedAt != nil {
		if err := s.revokeFamily(
			ctx,
			tx,
			familyID,
		); err != nil {
			return nil, err
		}

		if err := s.recordReplayEvent(
			ctx,
			tx,
			userID,
			ip,
			userAgent,
		); err != nil {
			return nil, err
		}

		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf(
				"commit refresh replay response: %w",
				err,
			)
		}

		return nil, ErrRefreshTokenReplay
	}

	if !expiresAt.After(time.Now().UTC()) {
		_, err = tx.Exec(
			ctx,
			`
			UPDATE user_sessions
			SET
				revoked_at = now(),
				revoked_reason = 'expired'
			WHERE id = $1
			`,
			oldSessionID,
		)

		if err != nil {
			return nil, fmt.Errorf(
				"revoke expired refresh session: %w",
				err,
			)
		}

		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf(
				"commit expired refresh session: %w",
				err,
			)
		}

		return nil, ErrInvalidRefreshToken
	}

	newTokenHash := tokenutil.Hash(newRefreshToken)

	newExpiresAt := time.Now().UTC().Add(ttl)

	var newSessionID uuid.UUID

	const createSession = `
		INSERT INTO user_sessions (
			user_id,
			refresh_token_hash,
			ip_address,
			user_agent,
			expires_at,
			family_id
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`

	err = tx.QueryRow(
		ctx,
		createSession,
		userID,
		newTokenHash,
		ip,
		userAgent,
		newExpiresAt,
		familyID,
	).Scan(&newSessionID)

	if err != nil {
		return nil, fmt.Errorf(
			"create rotated refresh session: %w",
			err,
		)
	}

	const revokeOldSession = `
		UPDATE user_sessions
		SET
			revoked_at = now(),
			revoked_reason = 'rotated',
			replaced_by_session_id = $2
		WHERE id = $1
	`

	if _, err := tx.Exec(
		ctx,
		revokeOldSession,
		oldSessionID,
		newSessionID,
	); err != nil {
		return nil, fmt.Errorf(
			"revoke old refresh session: %w",
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf(
			"commit refresh rotation: %w",
			err,
		)
	}

	return &Session{
		ID:        newSessionID,
		UserID:    userID,
		FamilyID:  familyID,
		ExpiresAt: newExpiresAt,
	}, nil
}

func (s *SessionService) revokeFamily(
	ctx context.Context,
	tx pgx.Tx,
	familyID uuid.UUID,
) error {
	const query = `
		UPDATE user_sessions
		SET
			revoked_at = COALESCE(revoked_at, now()),
			revoked_reason = CASE
				WHEN revoked_at IS NULL
				THEN 'replay_detected'
				ELSE revoked_reason
			END
		WHERE family_id = $1
		  AND revoked_at IS NULL
	`

	if _, err := tx.Exec(
		ctx,
		query,
		familyID,
	); err != nil {
		return fmt.Errorf(
			"revoke refresh token family: %w",
			err,
		)
	}

	return nil
}

func (s *SessionService) recordReplayEvent(
	ctx context.Context,
	tx pgx.Tx,
	userID uuid.UUID,
	ip net.IP,
	userAgent string,
) error {
	const query = `
		INSERT INTO security_events (
			user_id,
			event_type,
			ip_address,
			user_agent,
			metadata
		)
		VALUES (
			$1,
			'REFRESH_TOKEN_REUSE_DETECTED',
			$2,
			$3,
			'{}'
		)
	`

	if _, err := tx.Exec(
		ctx,
		query,
		userID,
		ip,
		userAgent,
	); err != nil {
		return fmt.Errorf(
			"record refresh token replay: %w",
			err,
		)
	}

	return nil
}

func (s *SessionService) Revoke(
	ctx context.Context,
	refreshToken string,
) error {
	if refreshToken == "" {
		return nil
	}

	tokenHash := tokenutil.Hash(refreshToken)

	const query = `
		UPDATE user_sessions
		SET
			revoked_at = COALESCE(revoked_at, now()),
			revoked_reason = CASE
				WHEN revoked_at IS NULL
				THEN 'logout'
				ELSE revoked_reason
			END
		WHERE refresh_token_hash = $1
	`

	if _, err := s.db.Exec(
		ctx,
		query,
		tokenHash,
	); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}

	return nil
}
