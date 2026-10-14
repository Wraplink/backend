package edgeaccess

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CurrentByUser(
	ctx context.Context,
	userID uuid.UUID,
) (*Assignment, error) {
	query := assignmentSelect() + `
		WHERE a.user_id = $1
		  AND a.state IN ('provisioning', 'active', 'suspended', 'releasing')
		ORDER BY a.created_at DESC
		LIMIT 1
	`
	item, err := scanAssignment(r.db.QueryRow(ctx, query, userID))
	if isNoRows(err) {
		return nil, ErrAssignmentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get current assignment: %w", err)
	}
	return item, nil
}

func (r *Repository) Provision(
	ctx context.Context,
	userID uuid.UUID,
	clientIP string,
	region string,
) (*Assignment, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin provisioning: %w", err)
	}
	defer rollback(tx, ctx)

	entitlement, err := lockUsableSubscription(ctx, tx, userID)
	if err != nil {
		return nil, err
	}

	existing, err := currentAssignmentForUpdate(ctx, tx, userID)
	if err != nil && !isNoRows(err) {
		return nil, fmt.Errorf("lock current assignment: %w", err)
	}
	if existing != nil {
		if existing.SubscriptionID != entitlement.ID || existing.State == StateReleasing {
			return nil, ErrNoEdgeCapacity
		}
		if existing.State == StateSuspended {
			return nil, ErrNoUsableSubscription
		}

		if existing.ClientIP != clientIP {
			existing, err = updateClientIP(ctx, tx, existing.ID, clientIP)
			if err != nil {
				return nil, err
			}
			edgeID, err := edgeIDForAssignment(ctx, tx, existing.ID)
			if err != nil {
				return nil, err
			}
			if _, err := insertSyncEvent(ctx, tx, edgeID, existing, EventAccessUpsert); err != nil {
				return nil, err
			}
		}
		if err := commit(tx, ctx, "existing provisioning"); err != nil {
			return nil, err
		}
		return existing, nil
	}

	candidate, err := selectCapacity(ctx, tx, strings.TrimSpace(region))
	if err != nil {
		return nil, err
	}

	var assignmentID uuid.UUID
	err = tx.QueryRow(
		ctx,
		`INSERT INTO network_access_assignments (
			user_id, subscription_id, edge_node_id, egress_ip_id, client_ip, state
		 ) VALUES ($1,$2,$3,$4,$5::inet,$6)
		 RETURNING id`,
		userID,
		entitlement.ID,
		candidate.EdgeID,
		candidate.EgressIPID,
		clientIP,
		StateProvisioning,
	).Scan(&assignmentID)
	if err != nil {
		return nil, fmt.Errorf("create assignment: %w", err)
	}

	_, err = tx.Exec(
		ctx,
		`UPDATE edge_public_ips
		 SET status = 'allocated', updated_at = now()
		 WHERE id = $1`,
		candidate.EgressIPID,
	)
	if err != nil {
		return nil, fmt.Errorf("reserve egress IP: %w", err)
	}

	assignment := &Assignment{
		ID:             assignmentID,
		SubscriptionID: entitlement.ID,
		EdgeCode:       candidate.EdgeCode,
		Region:         candidate.Region,
		EgressIP:       candidate.EgressIP,
		ClientIP:       clientIP,
		State:          StateProvisioning,
		AllocatedAt:    time.Now().UTC(),
		LastActivityAt: time.Now().UTC(),
	}
	if _, err := insertSyncEvent(ctx, tx, candidate.EdgeID, assignment, EventAccessUpsert); err != nil {
		return nil, err
	}

	if err := commit(tx, ctx, "provisioning"); err != nil {
		return nil, err
	}
	return assignment, nil
}

func (r *Repository) RequestRelease(
	ctx context.Context,
	userID uuid.UUID,
	reason string,
) (*Assignment, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin release: %w", err)
	}
	defer rollback(tx, ctx)

	assignment, err := currentAssignmentForUpdate(ctx, tx, userID)
	if isNoRows(err) {
		return nil, ErrAssignmentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock assignment for release: %w", err)
	}
	if assignment.State == StateReleasing {
		if err := commit(tx, ctx, "idempotent release"); err != nil {
			return nil, err
		}
		return assignment, nil
	}

	assignment, err = markReleasing(ctx, tx, assignment.ID, reason)
	if err != nil {
		return nil, err
	}
	edgeID, err := edgeIDForAssignment(ctx, tx, assignment.ID)
	if err != nil {
		return nil, err
	}
	if _, err := insertSyncEvent(ctx, tx, edgeID, assignment, EventAccessRemove); err != nil {
		return nil, err
	}
	if err := commit(tx, ctx, "release"); err != nil {
		return nil, err
	}
	return assignment, nil
}

func lockUsableSubscription(
	ctx context.Context,
	tx pgx.Tx,
	userID uuid.UUID,
) (*subscriptionEntitlement, error) {
	var item subscriptionEntitlement
	err := tx.QueryRow(
		ctx,
		`SELECT id, user_id, data_limit_bytes, used_bytes, expires_at
		 FROM user_subscriptions
		 WHERE user_id = $1
		   AND status = 'active'
		   AND starts_at <= now()
		   AND expires_at > now()
		   AND used_bytes < data_limit_bytes
		 ORDER BY starts_at DESC
		 LIMIT 1
		 FOR UPDATE`,
		userID,
	).Scan(&item.ID, &item.UserID, &item.DataLimitBytes, &item.UsedBytes, &item.ExpiresAt)
	if isNoRows(err) {
		return nil, ErrNoUsableSubscription
	}
	if err != nil {
		return nil, fmt.Errorf("lock usable subscription: %w", err)
	}
	return &item, nil
}

func currentAssignmentForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	userID uuid.UUID,
) (*Assignment, error) {
	query := assignmentSelect() + `
		WHERE a.user_id = $1
		  AND a.state IN ('provisioning', 'active', 'suspended', 'releasing')
		ORDER BY a.created_at DESC
		LIMIT 1
		FOR UPDATE OF a
	`
	return scanAssignment(tx.QueryRow(ctx, query, userID))
}

func updateClientIP(
	ctx context.Context,
	tx pgx.Tx,
	assignmentID uuid.UUID,
	clientIP string,
) (*Assignment, error) {
	_, err := tx.Exec(
		ctx,
		`UPDATE network_access_assignments
		 SET client_ip = $2::inet,
		     state = 'provisioning',
		     reason = NULL,
		     last_activity_at = now(),
		     updated_at = now()
		 WHERE id = $1`,
		assignmentID,
		clientIP,
	)
	if err != nil {
		return nil, fmt.Errorf("update assignment client IP: %w", err)
	}
	query := assignmentSelect() + ` WHERE a.id = $1`
	item, err := scanAssignment(tx.QueryRow(ctx, query, assignmentID))
	if err != nil {
		return nil, fmt.Errorf("reload updated assignment: %w", err)
	}
	return item, nil
}

func selectCapacity(
	ctx context.Context,
	tx pgx.Tx,
	region string,
) (*allocationCandidate, error) {
	var c allocationCandidate
	err := tx.QueryRow(
		ctx,
		`SELECT e.id, e.code, e.region, ip.id, host(ip.ip)
		 FROM edge_public_ips ip
		 JOIN edge_nodes e ON e.id = ip.edge_node_id
		 WHERE ip.status = 'available'
		   AND e.enabled = true
		   AND e.status = 'online'
		   AND ($1 = '' OR e.region = $1)
		 ORDER BY (
		     SELECT COUNT(*)
		     FROM network_access_assignments a
		     WHERE a.edge_node_id = e.id
		       AND a.state IN ('provisioning', 'active', 'suspended', 'releasing')
		 ) ASC,
		 e.last_seen_at DESC,
		 ip.ip
		 LIMIT 1
		 FOR UPDATE OF ip SKIP LOCKED`,
		region,
	).Scan(&c.EdgeID, &c.EdgeCode, &c.Region, &c.EgressIPID, &c.EgressIP)
	if isNoRows(err) {
		return nil, ErrNoEdgeCapacity
	}
	if err != nil {
		return nil, fmt.Errorf("select edge capacity: %w", err)
	}
	return &c, nil
}

func edgeIDForAssignment(ctx context.Context, tx pgx.Tx, assignmentID uuid.UUID) (uuid.UUID, error) {
	var edgeID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT edge_node_id FROM network_access_assignments WHERE id = $1`, assignmentID).Scan(&edgeID); err != nil {
		return uuid.Nil, fmt.Errorf("load assignment edge: %w", err)
	}
	return edgeID, nil
}

func markReleasing(ctx context.Context, tx pgx.Tx, assignmentID uuid.UUID, reason string) (*Assignment, error) {
	_, err := tx.Exec(
		ctx,
		`UPDATE network_access_assignments
		 SET state = 'releasing', reason = $2, updated_at = now()
		 WHERE id = $1
		   AND state <> 'released'`,
		assignmentID,
		reason,
	)
	if err != nil {
		return nil, fmt.Errorf("mark assignment releasing: %w", err)
	}
	query := assignmentSelect() + ` WHERE a.id = $1`
	item, err := scanAssignment(tx.QueryRow(ctx, query, assignmentID))
	if err != nil {
		return nil, fmt.Errorf("reload releasing assignment: %w", err)
	}
	return item, nil
}
