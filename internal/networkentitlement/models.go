package networkentitlement

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNoUsableSubscription = errors.New("no usable subscription")
	ErrEntitlementNotFound  = errors.New("network entitlement not found")
	ErrClientIPInUse        = errors.New("client IP is already in use")
	ErrInvalidClientIP      = errors.New("invalid client IP")
)

type Entitlement struct {
	ID             uuid.UUID `json:"id"`
	SubscriptionID uuid.UUID `json:"subscriptionId"`
	ClientIP       string    `json:"clientIp"`
	Status         string    `json:"status"`
	PlanCode       string    `json:"planCode"`
	DataLimitBytes int64     `json:"dataLimitBytes"`
	UsedBytes      int64     `json:"usedBytes"`
	RemainingBytes int64     `json:"remainingBytes"`
	StartsAt       time.Time `json:"startsAt"`
	ExpiresAt      time.Time `json:"expiresAt"`
	ActivatedAt    time.Time `json:"activatedAt"`
}

type SnapshotItem struct {
	EntitlementID  uuid.UUID `json:"entitlementId"`
	UserID         uuid.UUID `json:"userId"`
	SubscriptionID uuid.UUID `json:"subscriptionId"`
	ClientIP       string    `json:"clientIp"`
	PlanCode       string    `json:"planCode"`
	DataLimitBytes int64     `json:"dataLimitBytes"`
	UsedBytes      int64     `json:"usedBytes"`
	RemainingBytes int64     `json:"remainingBytes"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

type Snapshot struct {
	GeneratedAt  time.Time      `json:"generatedAt"`
	Entitlements []SnapshotItem `json:"entitlements"`
}
