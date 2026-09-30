package auth

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	tokenutil "backend/internal/token"
)

const refreshTokenBytes = 32

type SessionService struct {
	db *pgxpool.Pool
}

func NewSessionService(
	db *pgxpool.Pool,
) *SessionService {
	return &SessionService{
		db: db,
	}
}

type Session struct {
	ID           uuid.UUID
	RefreshToken string
	ExpiresAt    time.Time
}

func (s *SessionService) Create(
	ctx context.Context,
	userID uuid.UUID,
	ip net.IP,
	userAgent string,
	ttl time.Duration,
) (*Session, error) {
	refreshToken, err := tokenutil.Generate(
		refreshTokenBytes,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"generate refresh token: %w",
			err,
		)
	}

	tokenHash := tokenutil.Hash(refreshToken)

	expiresAt := time.Now().UTC().Add(ttl)

	var sessionID uuid.UUID

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

	err = s.db.QueryRow(
		ctx,
		query,
		userID,
		tokenHash,
		ip,
		userAgent,
		expiresAt,
	).Scan(&sessionID)

	if err != nil {
		return nil, fmt.Errorf(
			"create session: %w",
			err,
		)
	}

	return &Session{
		ID:           sessionID,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
	}, nil
}
