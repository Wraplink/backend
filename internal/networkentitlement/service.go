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
	mode, err := s.repository.GetMode(ctx, userID)
	if err != nil {
		return nil, err
	}
	entitlement, err := s.repository.Activate(ctx, userID, clientIP)
	if err != nil {
		return nil, err
	}
	entitlement.ConnectionMode = mode
	return entitlement, nil
}

func (s *Service) Current(
	ctx context.Context,
	userID uuid.UUID,
) (*Entitlement, error) {
	return s.repository.Current(ctx, userID)
}

func (s *Service) GetMode(ctx context.Context, userID uuid.UUID) (ConnectionMode, error) {
	return s.repository.GetMode(ctx, userID)
}

func (s *Service) SetMode(ctx context.Context, userID uuid.UUID, mode ConnectionMode) error {
	if !mode.Valid() {
		return ErrInvalidConnectionMode
	}
	return s.repository.SetMode(ctx, userID, mode)
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
