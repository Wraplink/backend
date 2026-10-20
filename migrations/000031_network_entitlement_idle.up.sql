-- An entitlement expires after 30 minutes without a new usage report.
-- The edge usage reporter must send periodic traffic deltas for active users.
ALTER TABLE network_entitlements
    ADD COLUMN last_activity_at TIMESTAMPTZ NOT NULL DEFAULT now();

UPDATE network_entitlements
SET last_activity_at = activated_at
WHERE status = 'active';

CREATE INDEX idx_network_entitlements_active_activity
    ON network_entitlements(last_activity_at)
    WHERE status = 'active';
