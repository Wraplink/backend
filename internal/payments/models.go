package payments

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	MethodWallet = "wallet"

	StatusSucceeded = "succeeded"

	DirectionCredit = "credit"
	DirectionDebit  = "debit"

	ReasonSubscriptionPayment = "subscription_payment"
)

var (
	ErrSubscriptionNotFound      = errors.New("subscription not found")
	ErrSubscriptionNotPending    = errors.New("subscription is not pending payment")
	ErrActiveSubscriptionExists  = errors.New("active subscription already exists")
	ErrInsufficientWalletBalance = errors.New("insufficient wallet balance")
	ErrIdempotencyConflict       = errors.New("idempotency key conflict")
	ErrPaymentNotFound           = errors.New("payment not found")
	ErrInvalidCurrency           = errors.New("invalid currency")
	ErrInvalidSubscriptionID     = errors.New("invalid subscription ID")
	ErrInvalidIdempotencyKey     = errors.New("invalid idempotency key")
)

type Wallet struct {
	Currency     string     `json:"currency"`
	BalanceMinor int64      `json:"balanceMinor"`
	UpdatedAt    *time.Time `json:"updatedAt,omitempty"`
}

type WalletTransaction struct {
	ID                uuid.UUID  `json:"id"`
	Currency          string     `json:"currency"`
	PaymentID         *uuid.UUID `json:"paymentId,omitempty"`
	Direction         string     `json:"direction"`
	Reason            string     `json:"reason"`
	AmountMinor       int64      `json:"amountMinor"`
	BalanceAfterMinor int64      `json:"balanceAfterMinor"`
	CreatedAt         time.Time  `json:"createdAt"`
}

type Payment struct {
	ID             uuid.UUID  `json:"id"`
	SubscriptionID *uuid.UUID `json:"subscriptionId,omitempty"`
	Method         string     `json:"method"`
	AmountMinor    int64      `json:"amountMinor"`
	Currency       string     `json:"currency"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"createdAt"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
}

type PaymentResult struct {
	Payment            Payment   `json:"payment"`
	SubscriptionID     uuid.UUID `json:"subscriptionId"`
	SubscriptionStatus string    `json:"subscriptionStatus"`
	StartsAt           time.Time `json:"startsAt"`
	ExpiresAt          time.Time `json:"expiresAt"`
}
