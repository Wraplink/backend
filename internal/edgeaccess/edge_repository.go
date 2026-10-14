package edgeaccess

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) RegisterEdge(ctx context.Context, request RegisterEdgeRequest) (*EdgeInfo, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin edge registration: %w", err)
	}
	defer rollback(tx, ctx)

	metadata, err := json.Marshal(request.Metadata)
	if err != nil {
		return nil, fmt.Errorf("encode edge metadata: %w", err)
	}

	var edgeID uuid.UUID
	var info EdgeInfo
	err = tx.QueryRow(
		ctx,
		`INSERT INTO edge_nodes (code, region, status, enabled, last_seen_at, metadata)
		 VALUES ($1, $2, 'online', true, now(), $3::jsonb)
		 ON CONFLICT (code) DO UPDATE
		 SET region = EXCLUDED.region,
		     status = CASE WHEN edge_nodes.status = 'draining' THEN 'draining' ELSE 'online' END,
		     last_seen_at = now(),
		     metadata = EXCLUDED.metadata,
		     updated_at = now()
		 RETURNING id, code, region, status, enabled, last_seen_at`,
		request.Code,
		request.Region,
		metadata,
	).Scan(&edgeID, &info.Code, &info.Region, &info.Status, &info.Enabled, &info.LastSeenAt)
	if err != nil {
		return nil, fmt.Errorf("upsert edge node: %w", err)
	}

	for _, ip := range request.PublicIPs {
		var ipID uuid.UUID
		err = tx.QueryRow(
			ctx,
			`INSERT INTO edge_public_ips (edge_node_id, ip)
			 VALUES ($1, $2::inet)
			 ON CONFLICT (ip) DO UPDATE
			 SET updated_at = now()
			 WHERE edge_public_ips.edge_node_id = EXCLUDED.edge_node_id
			 RETURNING id`,
			edgeID,
			ip,
		).Scan(&ipID)
		if isNoRows(err) {
			return nil, ErrInvalidEdgeRequest
		}
		if err != nil {
			return nil, fmt.Errorf("upsert edge public IP: %w", err)
		}
	}

	if err := commit(tx, ctx, "edge registration"); err != nil {
		return nil, err
	}
	return &info, nil
}

func (r *Repository) Heartbeat(ctx context.Context, edgeCode string) (*EdgeInfo, error) {
	var info EdgeInfo
	err := r.db.QueryRow(
		ctx,
		`UPDATE edge_nodes
		 SET status = CASE WHEN status = 'draining' THEN 'draining' ELSE 'online' END,
		     last_seen_at = now(),
		     updated_at = now()
		 WHERE code = $1
		 RETURNING code, region, status, enabled, last_seen_at`,
		edgeCode,
	).Scan(&info.Code, &info.Region, &info.Status, &info.Enabled, &info.LastSeenAt)
	if isNoRows(err) {
		return nil, ErrEdgeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("edge heartbeat: %w", err)
	}
	return &info, nil
}

func (r *Repository) Sync(ctx context.Context, edgeCode string, after int64, limit int) (*SyncBatch, error) {
	var edgeID uuid.UUID
	if err := r.db.QueryRow(ctx, `SELECT id FROM edge_nodes WHERE code = $1`, edgeCode).Scan(&edgeID); isNoRows(err) {
		return nil, ErrEdgeNotFound
	} else if err != nil {
		return nil, fmt.Errorf("load edge for sync: %w", err)
	}

	var current int64
	if err := r.db.QueryRow(
		ctx,
		`SELECT COALESCE(MAX(revision), 0) FROM edge_sync_events WHERE edge_node_id = $1`,
		edgeID,
	).Scan(&current); err != nil {
		return nil, fmt.Errorf("load current edge revision: %w", err)
	}

	rows, err := r.db.Query(
		ctx,
		`SELECT revision, event_type, access_id, payload, created_at
		 FROM edge_sync_events
		 WHERE edge_node_id = $1
		   AND revision > $2
		 ORDER BY revision
		 LIMIT $3`,
		edgeID,
		after,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list edge sync events: %w", err)
	}
	defer rows.Close()

	events := make([]SyncEvent, 0)
	for rows.Next() {
		var event SyncEvent
		var payload []byte
		if err := rows.Scan(&event.Revision, &event.EventType, &event.AccessID, &payload, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan edge sync event: %w", err)
		}
		if err := json.Unmarshal(payload, &event.Payload); err != nil {
			return nil, fmt.Errorf("decode edge sync event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate edge sync events: %w", err)
	}

	return &SyncBatch{
		EdgeCode:        edgeCode,
		AfterRevision:   after,
		CurrentRevision: current,
		Events:          events,
	}, nil
}

func (r *Repository) Acknowledge(ctx context.Context, edgeCode string, request AckRequest) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin edge acknowledgement: %w", err)
	}
	defer rollback(tx, ctx)

	var eventType string
	var accessID uuid.UUID
	var egressIPID uuid.UUID
	var state string
	err = tx.QueryRow(
		ctx,
		`SELECT ev.event_type, ev.access_id, a.egress_ip_id, a.state
		 FROM edge_sync_events ev
		 JOIN edge_nodes e ON e.id = ev.edge_node_id
		 JOIN network_access_assignments a ON a.id = ev.access_id
		 WHERE ev.revision = $1
		   AND e.code = $2
		 FOR UPDATE OF ev, a`,
		request.Revision,
		edgeCode,
	).Scan(&eventType, &accessID, &egressIPID, &state)
	if isNoRows(err) {
		return ErrRevisionNotFound
	}
	if err != nil {
		return fmt.Errorf("load sync event for acknowledgement: %w", err)
	}

	if !request.Success {
		_, err = tx.Exec(
			ctx,
			`UPDATE network_access_assignments
			 SET reason = $2, updated_at = now()
			 WHERE id = $1`,
			accessID,
			request.Error,
		)
		if err != nil {
			return fmt.Errorf("record edge apply failure: %w", err)
		}
		return commit(tx, ctx, "failed edge acknowledgement")
	}

	_, err = tx.Exec(ctx, `UPDATE edge_sync_events SET acknowledged_at = now() WHERE revision = $1`, request.Revision)
	if err != nil {
		return fmt.Errorf("acknowledge sync event: %w", err)
	}

	switch eventType {
	case EventAccessUpsert:
		_, err = tx.Exec(
			ctx,
			`UPDATE network_access_assignments
			 SET state = 'active', reason = NULL, updated_at = now()
			 WHERE id = $1
			   AND state = 'provisioning'`,
			accessID,
		)
		if err != nil {
			return fmt.Errorf("activate assignment after edge acknowledgement: %w", err)
		}
	case EventAccessRemove:
		_, err = tx.Exec(
			ctx,
			`UPDATE network_access_assignments
			 SET state = 'released', released_at = now(), updated_at = now()
			 WHERE id = $1
			   AND state IN ('releasing', 'suspended')`,
			accessID,
		)
		if err != nil {
			return fmt.Errorf("release assignment after edge acknowledgement: %w", err)
		}
		_, err = tx.Exec(
			ctx,
			`UPDATE edge_public_ips
			 SET status = 'available', updated_at = now()
			 WHERE id = $1
			   AND status = 'allocated'`,
			egressIPID,
		)
		if err != nil {
			return fmt.Errorf("release egress IP: %w", err)
		}
	}

	return commit(tx, ctx, "edge acknowledgement")
}

func (r *Repository) RecordActivity(ctx context.Context, edgeCode string, items []ActivityItem) error {
	var edgeID uuid.UUID
	if err := r.db.QueryRow(ctx, `SELECT id FROM edge_nodes WHERE code = $1`, edgeCode).Scan(&edgeID); isNoRows(err) {
		return ErrEdgeNotFound
	} else if err != nil {
		return fmt.Errorf("load edge for activity: %w", err)
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin edge activity update: %w", err)
	}
	defer rollback(tx, ctx)

	for _, item := range items {
		at := item.At
		if at.IsZero() {
			at = time.Now().UTC()
		}
		_, err := tx.Exec(
			ctx,
			`UPDATE network_access_assignments
			 SET last_activity_at = GREATEST(last_activity_at, $3), updated_at = now()
			 WHERE id = $1
			   AND edge_node_id = $2
			   AND state IN ('provisioning', 'active')`,
			item.AccessID,
			edgeID,
			at,
		)
		if err != nil {
			return fmt.Errorf("update edge activity: %w", err)
		}
	}

	return commit(tx, ctx, "edge activity")
}

func (r *Repository) Reconcile(ctx context.Context, idleAfter, edgeOfflineAfter time.Duration) (int, error) {
	if _, err := r.db.Exec(
		ctx,
		`UPDATE edge_nodes
		 SET status = 'offline', updated_at = now()
		 WHERE status = 'online'
		   AND last_seen_at < now() - make_interval(secs => $1)`,
		edgeOfflineAfter.Seconds(),
	); err != nil {
		return 0, fmt.Errorf("mark stale edges offline: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return 0, fmt.Errorf("begin access reconciliation: %w", err)
	}
	defer rollback(tx, ctx)

	rows, err := tx.Query(
		ctx,
		`SELECT a.id,
		        CASE
		          WHEN s.status <> 'active' OR s.expires_at <= now() THEN 'subscription_expired'
		          WHEN s.used_bytes >= s.data_limit_bytes THEN 'quota_exhausted'
		          ELSE 'idle_timeout'
		        END
		 FROM network_access_assignments a
		 JOIN user_subscriptions s ON s.id = a.subscription_id
		 WHERE a.state IN ('provisioning', 'active')
		   AND (
		       s.status <> 'active'
		       OR s.expires_at <= now()
		       OR s.used_bytes >= s.data_limit_bytes
		       OR a.last_activity_at < now() - make_interval(secs => $1)
		   )
		 ORDER BY a.updated_at
		 LIMIT 500
		 FOR UPDATE OF a SKIP LOCKED`,
		idleAfter.Seconds(),
	)
	if err != nil {
		return 0, fmt.Errorf("select assignments for reconciliation: %w", err)
	}

	type staleAssignment struct {
		ID     uuid.UUID
		Reason string
	}
	stale := make([]staleAssignment, 0)
	for rows.Next() {
		var item staleAssignment
		if err := rows.Scan(&item.ID, &item.Reason); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan stale assignment: %w", err)
		}
		stale = append(stale, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate stale assignments: %w", err)
	}
	rows.Close()

	for _, item := range stale {
		assignment, err := markReleasing(ctx, tx, item.ID, item.Reason)
		if err != nil {
			return 0, err
		}
		edgeID, err := edgeIDForAssignment(ctx, tx, assignment.ID)
		if err != nil {
			return 0, err
		}
		if _, err := insertSyncEvent(ctx, tx, edgeID, assignment, EventAccessRemove); err != nil {
			return 0, err
		}
	}

	if err := commit(tx, ctx, "access reconciliation"); err != nil {
		return 0, err
	}
	return len(stale), nil
}
