package payments

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func releaseDeletedAccountEmail(ctx context.Context, tx pgx.Tx, actor, target uuid.UUID, placeholder string) error {
	if _, err := tx.Exec(ctx, "UPDATE users SET email=$2, updated_at=now() WHERE id=$1", target, placeholder); err != nil {
		return fmt.Errorf("replace deleted account email: %w", err)
	}
	if _, err := tx.Exec(ctx, "DELETE FROM google_identities WHERE user_id=$1", target); err != nil {
		return fmt.Errorf("unlink Google identity: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO security_events (user_id, event_type, metadata)
		VALUES ($1, 'ADMIN_DELETED_ACCOUNT_EMAIL_RELEASED', jsonb_build_object('actorId', $2::text))`,
		target, actor.String()); err != nil {
		return fmt.Errorf("record deleted account email release: %w", err)
	}
	return nil
}
