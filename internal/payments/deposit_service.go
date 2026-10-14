package payments

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

func (s *Service) CreateDeposit(
	ctx context.Context,
	userID uuid.UUID,
	amountMinor int64,
	currency string,
	idempotencyKey string,
) (*Deposit, error) {
	if amountMinor <= 0 {
		return nil, ErrInvalidDepositAmount
	}

	currency, err := normalizeCurrency(currency)
	if err != nil {
		return nil, err
	}

	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 100 {
		return nil, ErrInvalidIdempotencyKey
	}

	return s.repository.CreateDeposit(
		ctx,
		userID,
		amountMinor,
		currency,
		ProviderDevelopment,
		idempotencyKey,
	)
}

func (s *Service) Deposits(
	ctx context.Context,
	userID uuid.UUID,
	limit int,
) ([]Deposit, error) {
	return s.repository.ListDeposits(ctx, userID, normalizeLimit(limit))
}

func (s *Service) Deposit(
	ctx context.Context,
	userID uuid.UUID,
	depositID uuid.UUID,
) (*Deposit, error) {
	if depositID == uuid.Nil {
		return nil, ErrInvalidDepositID
	}

	return s.repository.GetDeposit(ctx, userID, depositID)
}

func (s *Service) SettleDevelopmentDeposit(
	ctx context.Context,
	depositID uuid.UUID,
	providerReference string,
) (*DepositSettlementResult, error) {
	if depositID == uuid.Nil {
		return nil, ErrInvalidDepositID
	}

	providerReference = strings.TrimSpace(providerReference)
	if providerReference == "" || len(providerReference) > 200 {
		return nil, ErrInvalidProviderReference
	}

	return s.repository.SettleDevelopmentDeposit(ctx, depositID, providerReference)
}
