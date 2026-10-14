package payments

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

const (
	defaultListLimit = 50
	maxListLimit     = 100
)

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	if repository == nil {
		panic("payments: nil repository")
	}

	return &Service{repository: repository}
}

func (s *Service) Wallet(
	ctx context.Context,
	userID uuid.UUID,
	currency string,
) (*Wallet, error) {
	currency, err := normalizeCurrency(currency)
	if err != nil {
		return nil, err
	}

	return s.repository.GetWallet(ctx, userID, currency)
}

func (s *Service) Transactions(
	ctx context.Context,
	userID uuid.UUID,
	currency string,
	limit int,
) ([]WalletTransaction, error) {
	currency, err := normalizeCurrency(currency)
	if err != nil {
		return nil, err
	}

	return s.repository.ListTransactions(ctx, userID, currency, normalizeLimit(limit))
}

func (s *Service) Payments(
	ctx context.Context,
	userID uuid.UUID,
	limit int,
) ([]Payment, error) {
	return s.repository.ListPayments(ctx, userID, normalizeLimit(limit))
}

func (s *Service) Payment(
	ctx context.Context,
	userID uuid.UUID,
	paymentID uuid.UUID,
) (*Payment, error) {
	if paymentID == uuid.Nil {
		return nil, ErrPaymentNotFound
	}

	return s.repository.GetPayment(ctx, userID, paymentID)
}

func (s *Service) PaySubscriptionWithWallet(
	ctx context.Context,
	userID uuid.UUID,
	subscriptionID uuid.UUID,
	idempotencyKey string,
) (*PaymentResult, error) {
	if subscriptionID == uuid.Nil {
		return nil, ErrInvalidSubscriptionID
	}

	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 100 {
		return nil, ErrInvalidIdempotencyKey
	}

	return s.repository.PaySubscriptionWithWallet(
		ctx,
		userID,
		subscriptionID,
		idempotencyKey,
	)
}

func normalizeCurrency(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 3 {
		return "", ErrInvalidCurrency
	}

	for _, ch := range value {
		if ch < 'A' || ch > 'Z' {
			return "", ErrInvalidCurrency
		}
	}

	return value, nil
}

func normalizeLimit(value int) int {
	if value <= 0 {
		return defaultListLimit
	}
	if value > maxListLimit {
		return maxListLimit
	}
	return value
}
