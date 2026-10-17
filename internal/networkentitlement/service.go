package networkentitlement

import (
	"context"
	"net"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	if repository == nil {
		panic("networkentitlement: nil repository")
	}

	return &Service{repository: repository}
}

func (s *Service) Activate(
	ctx context.Context,
	userID uuid.UUID,
	clientIP net.IP,
) (*Entitlement, error) {
	return s.repository.Activate(
		ctx,
		userID,
		clientIP,
	)
}

func (s *Service) Current(
	ctx context.Context,
	userID uuid.UUID,
) (*Entitlement, error) {
	return s.repository.Current(ctx, userID)
}

func (s *Service) Deactivate(
	ctx context.Context,
	userID uuid.UUID,
) error {
	return s.repository.Deactivate(ctx, userID)
}

func (s *Service) Snapshot(
	ctx context.Context,
) (*Snapshot, error) {
	items, err := s.repository.Snapshot(ctx)
	if err != nil {
		return nil, err
	}

	return &Snapshot{
		GeneratedAt:  time.Now().UTC(),
		Entitlements: items,
	}, nil
}
