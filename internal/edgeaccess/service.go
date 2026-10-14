package edgeaccess

import (
	"context"
	"crypto/subtle"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var edgeCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

type Service struct {
	repository       *Repository
	syncKey          string
	idleAfter        time.Duration
	edgeOfflineAfter time.Duration
}

func NewService(
	repository *Repository,
	syncKey string,
	idleAfter time.Duration,
	edgeOfflineAfter time.Duration,
) *Service {
	if repository == nil {
		panic("edgeaccess: nil repository")
	}
	return &Service{
		repository:       repository,
		syncKey:          strings.TrimSpace(syncKey),
		idleAfter:        idleAfter,
		edgeOfflineAfter: edgeOfflineAfter,
	}
}

func (s *Service) InternalEnabled() bool {
	return s.syncKey != ""
}

func (s *Service) Authorize(provided string) bool {
	if !s.InternalEnabled() {
		return false
	}
	provided = strings.TrimSpace(provided)
	return subtle.ConstantTimeCompare([]byte(provided), []byte(s.syncKey)) == 1
}

func (s *Service) Provision(
	ctx context.Context,
	userID uuid.UUID,
	clientIP string,
	region string,
) (*Assignment, error) {
	ip := net.ParseIP(strings.TrimSpace(clientIP))
	if ip == nil {
		return nil, ErrInvalidClientIP
	}
	return s.repository.Provision(ctx, userID, ip.String(), strings.TrimSpace(region))
}

func (s *Service) Current(ctx context.Context, userID uuid.UUID) (*Assignment, error) {
	return s.repository.CurrentByUser(ctx, userID)
}

func (s *Service) Release(ctx context.Context, userID uuid.UUID) (*Assignment, error) {
	return s.repository.RequestRelease(ctx, userID, "user_request")
}

func (s *Service) RegisterEdge(ctx context.Context, request RegisterEdgeRequest) (*EdgeInfo, error) {
	request.Code = strings.TrimSpace(strings.ToLower(request.Code))
	request.Region = strings.TrimSpace(strings.ToLower(request.Region))
	if !edgeCodePattern.MatchString(request.Code) || request.Region == "" || len(request.Region) > 64 {
		return nil, ErrInvalidEdgeRequest
	}
	if len(request.PublicIPs) == 0 || len(request.PublicIPs) > 256 {
		return nil, ErrInvalidEdgeRequest
	}
	seen := make(map[string]struct{}, len(request.PublicIPs))
	for i, raw := range request.PublicIPs {
		ip := net.ParseIP(strings.TrimSpace(raw))
		if ip == nil {
			return nil, ErrInvalidEdgeRequest
		}
		value := ip.String()
		if _, exists := seen[value]; exists {
			return nil, ErrInvalidEdgeRequest
		}
		seen[value] = struct{}{}
		request.PublicIPs[i] = value
	}
	if request.Metadata == nil {
		request.Metadata = map[string]any{}
	}
	return s.repository.RegisterEdge(ctx, request)
}

func (s *Service) Heartbeat(ctx context.Context, edgeCode string) (*EdgeInfo, error) {
	edgeCode = strings.TrimSpace(strings.ToLower(edgeCode))
	if !edgeCodePattern.MatchString(edgeCode) {
		return nil, ErrInvalidEdgeRequest
	}
	return s.repository.Heartbeat(ctx, edgeCode)
}

func (s *Service) Sync(ctx context.Context, edgeCode string, after int64, limit int) (*SyncBatch, error) {
	edgeCode = strings.TrimSpace(strings.ToLower(edgeCode))
	if !edgeCodePattern.MatchString(edgeCode) || after < 0 {
		return nil, ErrInvalidEdgeRequest
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	return s.repository.Sync(ctx, edgeCode, after, limit)
}

func (s *Service) Acknowledge(ctx context.Context, edgeCode string, request AckRequest) error {
	edgeCode = strings.TrimSpace(strings.ToLower(edgeCode))
	if !edgeCodePattern.MatchString(edgeCode) || request.Revision <= 0 {
		return ErrInvalidEdgeRequest
	}
	if len(request.Error) > 500 {
		request.Error = request.Error[:500]
	}
	return s.repository.Acknowledge(ctx, edgeCode, request)
}

func (s *Service) RecordActivity(ctx context.Context, edgeCode string, request ActivityRequest) error {
	edgeCode = strings.TrimSpace(strings.ToLower(edgeCode))
	if !edgeCodePattern.MatchString(edgeCode) || len(request.Items) == 0 || len(request.Items) > 1000 {
		return ErrInvalidEdgeRequest
	}
	for _, item := range request.Items {
		if item.AccessID == uuid.Nil {
			return ErrInvalidEdgeRequest
		}
	}
	return s.repository.RecordActivity(ctx, edgeCode, request.Items)
}

func (s *Service) Reconcile(ctx context.Context) (int, error) {
	return s.repository.Reconcile(ctx, s.idleAfter, s.edgeOfflineAfter)
}
