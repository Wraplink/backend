package payments

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	ProviderDevelopment = "development"
	ReasonWalletDeposit = "wallet_deposit"
)

var (
	ErrDepositNotFound              = errors.New("deposit not found")
	ErrDepositNotPending            = errors.New("deposit is not pending")
	ErrInvalidDepositAmount         = errors.New("invalid deposit amount")
	ErrInvalidDepositID             = errors.New("invalid deposit ID")
	ErrInvalidProviderReference     = errors.New("invalid provider reference")
	ErrDevelopmentSettlementDisabled = errors.New("development settlement is disabled")
)

type Deposit struct {
	ID                uuid.UUID  `json:"id"`
	AmountMinor       int64      `json:"amountMinor"`
	Currency          string     `json:"currency"`
	Provider          string     `json:"provider"`
	Status            string     `json:"status"`
	ProviderReference *string    `json:"providerReference,omitempty"`
	PaymentID         *uuid.UUID `json:"paymentId,omitempty"`
	CompletedAt       *time.Time `json:"completedAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

type DepositSettlementResult struct {
	Deposit Deposit `json:"deposit"`
	Wallet  Wallet  `json:"wallet"`
}
