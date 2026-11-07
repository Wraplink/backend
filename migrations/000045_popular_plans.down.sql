DROP INDEX IF EXISTS idx_plans_popular;
ALTER TABLE plans DROP COLUMN IF EXISTS is_popular;
