package edgeaccess

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	StateProvisioning = "provisioning"
	StateActive       = "active"
	StateSuspended    = "suspended"
	StateReleasing    = "releasing"
	StateReleased     = "released"

	EventAccessUpsert = "access.upsert"
	EventAccessRemove = "access.remove"
)

var (
	ErrNoUsableSubscription = errors.New("no usable subscription")
	ErrNoEdgeCapacity        = errors.New("no edge capacity available")
	ErrAssignmentNotFound    = errors.New("network assignment not found")
	ErrEdgeNotFound          = errors.New("edge node not found")
	ErrInvalidEdgeRequest    = errors.New("invalid edge request")
	ErrInvalidClientIP       = errors.New("invalid client IP")
	ErrInternalSyncDisabled  = errors.New("edge sync is disabled")
	ErrUnauthorizedEdge      = errors.New("unauthorized edge request")
	ErrRevisionNotFound      = errors.New("sync revision not found")
)

type Assignment struct {
	ID             uuid.UUID  `json:"id"`
	SubscriptionID uuid.UUID  `json:"subscriptionId"`
	EdgeCode       string     `json:"edgeCode"`
	Region         string     `json:"region"`
	EgressIP       string     `json:"egressIp"`
	ClientIP       string     `json:"clientIp"`
	State          string     `json:"state"`
	Reason         *string    `json:"reason,omitempty"`
	AllocatedAt    time.Time  `json:"allocatedAt"`
	LastActivityAt time.Time  `json:"lastActivityAt"`
	ReleasedAt     *time.Time `json:"releasedAt,omitempty"`
}

type ProvisionRequest struct {
	Region string `json:"region"`
}

type RegisterEdgeRequest struct {
	Code      string         `json:"code"`
	Region    string         `json:"region"`
	PublicIPs []string       `json:"publicIps"`
	Metadata  map[string]any `json:"metadata"`
}

type EdgeInfo struct {
	Code       string    `json:"code"`
	Region     string    `json:"region"`
	Status     string    `json:"status"`
	Enabled    bool      `json:"enabled"`
	LastSeenAt time.Time `json:"lastSeenAt"`
}

type SyncEvent struct {
	Revision  int64          `json:"revision"`
	EventType string         `json:"eventType"`
	AccessID  uuid.UUID      `json:"accessId"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"createdAt"`
}

type SyncBatch struct {
	EdgeCode        string      `json:"edgeCode"`
	AfterRevision   int64       `json:"afterRevision"`
	CurrentRevision int64       `json:"currentRevision"`
	Events          []SyncEvent `json:"events"`
}

type AckRequest struct {
	Revision int64  `json:"revision"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

type ActivityItem struct {
	AccessID uuid.UUID `json:"accessId"`
	At       time.Time `json:"at"`
}

type ActivityRequest struct {
	Items []ActivityItem `json:"items"`
}
