package subscriptions

import (
	"context"
	"errors"
	"strings"

	"backend/internal/plans"

	"github.com/google/uuid"
)

var (
	ErrPlanNotFound         = errors.New("plan not found")
	ErrSubscriptionNotFound = errors.New("subscription not found")
	ErrInvalidUserID        = errors.New("invalid user ID")
	ErrInvalidPlanCode      = errors.New("invalid plan code")
)

type Service struct {
	repository      *Repository
	plansRepository *plans.Repository
}

func NewService(
	repository *Repository,
	plansRepository *plans.Repository,
) *Service {
	if repository == nil {
		panic("subscriptions: nil repository")
	}
	if plansRepository == nil {
		panic("subscriptions: nil plans repository")
	}

	return &Service{
		repository:      repository,
		plansRepository: plansRepository,
	}
}

func (s *Service) CreatePending(
	ctx context.Context,
	userID uuid.UUID,
	planCode string,
) (*Subscription, error) {
	if userID == uuid.Nil {
		return nil, ErrInvalidUserID
	}

	planCode = strings.ToLower(strings.TrimSpace(planCode))
	if planCode == "" {
		return nil, ErrInvalidPlanCode
	}

	plan, err := s.plansRepository.GetActiveByCode(ctx, planCode)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, ErrPlanNotFound
	}

	return s.repository.CreatePending(
		ctx,
		CreatePendingParams{
			UserID:         userID,
			PlanID:         plan.ID,
			PlanCode:       plan.Code,
			PlanName:       plan.Name,
			PriceMinor:     plan.PriceMinor,
			Currency:       plan.Currency,
			DurationDays:   plan.DurationDays,
			DataLimitBytes: plan.DataLimitBytes,
		},
	)
}

func (s *Service) List(
	ctx context.Context,
	userID uuid.UUID,
) ([]Subscription, error) {
	if userID == uuid.Nil {
		return nil, ErrInvalidUserID
	}

	return s.repository.ListByUser(ctx, userID)
}

func (s *Service) Current(
	ctx context.Context,
	userID uuid.UUID,
) (*Subscription, error) {
	if userID == uuid.Nil {
		return nil, ErrInvalidUserID
	}

	return s.repository.Current(ctx, userID)
}

func (s *Service) CancelPending(
	ctx context.Context,
	userID uuid.UUID,
	subscriptionID uuid.UUID,
) error {
	if userID == uuid.Nil {
		return ErrInvalidUserID
	}
	if subscriptionID == uuid.Nil {
		return ErrSubscriptionNotFound
	}

	cancelled, err := s.repository.CancelPending(ctx, userID, subscriptionID)
	if err != nil {
		return err
	}
	if !cancelled {
		// Do not disclose whether the subscription belongs to another user or
		// has a status that cannot be cancelled through this endpoint.
		return ErrSubscriptionNotFound
	}

	return nil
}
