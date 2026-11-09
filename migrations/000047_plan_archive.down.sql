DROP INDEX IF EXISTS idx_plans_archived_at;
ALTER TABLE plans DROP COLUMN archived_at;
