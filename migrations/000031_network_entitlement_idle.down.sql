DROP INDEX IF EXISTS idx_network_entitlements_active_activity;
ALTER TABLE network_entitlements DROP COLUMN IF EXISTS last_activity_at;
