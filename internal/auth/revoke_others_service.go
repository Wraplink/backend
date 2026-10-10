package auth

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type OtherSessionRevoker interface {
	RevokeOthers(
		ctx context.Context,
		userID uuid.UUID,
		currentSessionID uuid.UUID,
	) (int64, error)
}

type RevokeOthersService struct {
	repository OtherSessionRevoker
}

func NewRevokeOthersService(
	repository OtherSessionRevoker,
) *RevokeOthersService {
	return &RevokeOthersService{
		repository: repository,
	}
}

func (s *RevokeOthersService) Revoke(
	ctx context.Context,
	userID uuid.UUID,
	currentSessionID uuid.UUID,
) (int64, error) {
	if userID == uuid.Nil || currentSessionID == uuid.Nil {
		return 0, ErrInvalidSessionID
	}

	count, err := s.repository.RevokeOthers(
		ctx,
		userID,
		currentSessionID,
	)
	if err != nil {
		return 0, fmt.Errorf("revoke other sessions: %w", err)
	}

	return count, nil
}
