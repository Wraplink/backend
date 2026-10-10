package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var ErrInvalidSessionID = errors.New("invalid session ID")

type SessionManagementService struct {
	repository *SessionRepository
}

func NewSessionManagementService(
	repository *SessionRepository,
) *SessionManagementService {
	if repository == nil {
		panic("auth: nil session repository")
	}

	return &SessionManagementService{
		repository: repository,
	}
}

func (s *SessionManagementService) ListActive(
	ctx context.Context,
	userID uuid.UUID,
) ([]ManagedSession, error) {
	if userID == uuid.Nil {
		return nil, errors.New("invalid user ID")
	}

	return s.repository.ListActive(ctx, userID)
}

func (s *SessionManagementService) Revoke(
	ctx context.Context,
	userID uuid.UUID,
	sessionID uuid.UUID,
) error {
	if userID == uuid.Nil {
		return errors.New("invalid user ID")
	}

	if sessionID == uuid.Nil {
		return ErrInvalidSessionID
	}

	revoked, err := s.repository.RevokeByID(
		ctx,
		userID,
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}

	if !revoked {
		// Do not disclose whether the session exists
		// or belongs to another user.
		return ErrInvalidSessionID
	}

	return nil
}
