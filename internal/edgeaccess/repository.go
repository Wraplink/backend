package edgeaccess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	if db == nil {
		panic("edgeaccess: nil database")
	}
	return &Repository{db: db}
}

type allocationCandidate struct {
	EdgeID     uuid.UUID
	EdgeCode   string
	Region     string
	EgressIPID uuid.UUID
	EgressIP   string
}

type subscriptionEntitlement struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	DataLimitBytes int64
	UsedBytes      int64
	ExpiresAt      time.Time
}

func scanAssignment(row pgx.Row) (*Assignment, error) {
	var item Assignment
	err := row.Scan(
		&item.ID,
		&item.SubscriptionID,
		&item.EdgeCode,
		&item.Region,
		&item.EgressIP,
		&item.ClientIP,
		&item.State,
		&item.Reason,
		&item.AllocatedAt,
		&item.LastActivityAt,
		&item.ReleasedAt,
	)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func assignmentSelect() string {
	return `
		SELECT
			a.id,
			a.subscription_id,
			e.code,
			e.region,
			host(ip.ip),
			host(a.client_ip),
			a.state,
			a.reason,
			a.allocated_at,
			a.last_activity_at,
			a.released_at
		FROM network_access_assignments a
		JOIN edge_nodes e ON e.id = a.edge_node_id
		JOIN edge_public_ips ip ON ip.id = a.egress_ip_id
	`
}

func insertSyncEvent(
	ctx context.Context,
	tx pgx.Tx,
	edgeID uuid.UUID,
	assignment *Assignment,
	eventType string,
) (int64, error) {
	payload := map[string]any{
		"accessId":       assignment.ID.String(),
		"subscriptionId": assignment.SubscriptionID.String(),
		"clientIp":       assignment.ClientIP,
		"egressIp":       assignment.EgressIP,
		"state":          assignment.State,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("encode sync event: %w", err)
	}

	var revision int64
	err = tx.QueryRow(
		ctx,
		`INSERT INTO edge_sync_events (edge_node_id, access_id, event_type, payload)
		 VALUES ($1, $2, $3, $4::jsonb)
		 RETURNING revision`,
		edgeID,
		assignment.ID,
		eventType,
		encoded,
	).Scan(&revision)
	if err != nil {
		return 0, fmt.Errorf("insert sync event: %w", err)
	}
	return revision, nil
}

func commit(tx pgx.Tx, ctx context.Context, operation string) error {
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit %s: %w", operation, err)
	}
	return nil
}

func rollback(tx pgx.Tx, ctx context.Context) {
	_ = tx.Rollback(ctx)
}

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
