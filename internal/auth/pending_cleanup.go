package auth

import (
    "context"
    "fmt"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
)

// CleanupPendingRegistrations deletes only old, unverified, pending registrations.
// A transaction rolls back entirely if any other table still references a candidate.
// No CASCADE is used: unexpected financial/audit dependencies must never be deleted.
func CleanupPendingRegistrations(ctx context.Context, db *pgxpool.Pool, retention time.Duration) (int64, error) {
    if retention < 24*time.Hour { return 0, fmt.Errorf("pending retention must be at least 24 hours") }
    tx, err := db.Begin(ctx)
    if err != nil { return 0, err }
    defer tx.Rollback(ctx)

    _, err = tx.Exec(ctx, `CREATE TEMP TABLE IF NOT EXISTS pending_registration_cleanup_ids
        (id uuid PRIMARY KEY) ON COMMIT DROP`)
    if err != nil { return 0, err }
    _, err = tx.Exec(ctx, `INSERT INTO pending_registration_cleanup_ids(id)
        SELECT id FROM users
        WHERE account_status = 'pending' AND email_verified = false
          AND created_at < now() - ($1::double precision * interval '1 second')
        ORDER BY created_at LIMIT 100 FOR UPDATE SKIP LOCKED`, retention.Seconds())
    if err != nil { return 0, err }

    for _, table := range []string{"email_verifications", "user_credentials", "legal_acceptances", "security_events"} {
        _, err = tx.Exec(ctx, "DELETE FROM "+table+" WHERE user_id IN (SELECT id FROM pending_registration_cleanup_ids)")
        if err != nil { return 0, fmt.Errorf("cleanup %s: %w", table, err) }
    }
    tag, err := tx.Exec(ctx, `DELETE FROM users
        WHERE id IN (SELECT id FROM pending_registration_cleanup_ids)
          AND account_status = 'pending' AND email_verified = false`)
    if err != nil { return 0, fmt.Errorf("cleanup pending users: %w", err) }
    if err := tx.Commit(ctx); err != nil { return 0, err }
    return tag.RowsAffected(), nil
}
