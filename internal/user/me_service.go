package user

import (
	"context"

	"github.com/google/uuid"
)

type MeService struct {
	repository *Repository
}

func NewMeService(
	repository *Repository,
) MeServiceInterface {
	return &MeService{
		repository: repository,
	}
}

func (s *MeService) Get(
	ctx context.Context,
	id uuid.UUID,
) (*User, error) {
	return s.repository.FindByID(ctx, id)
}

func (s *MeService) Update(
	ctx context.Context,
	id uuid.UUID,
	fullName string,
) (*User, error) {
	if err := s.repository.UpdateFullName(
		ctx,
		id,
		fullName,
	); err != nil {
		return nil, err
	}

	return s.repository.FindByID(
		ctx,
		id,
	)
}

func (s *MeService) GetAppearance(ctx context.Context,id uuid.UUID,theme *string) error {return s.repository.GetAppearance(ctx,id,theme)}
func (s *MeService) SetAppearance(ctx context.Context,id uuid.UUID,theme string) error {return s.repository.SetAppearance(ctx,id,theme)}
