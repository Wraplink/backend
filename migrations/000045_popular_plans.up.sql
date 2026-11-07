ALTER TABLE plans ADD COLUMN IF NOT EXISTS is_popular boolean NOT NULL DEFAULT false;
CREATE INDEX IF NOT EXISTS idx_plans_popular ON plans (sort_order, code) WHERE is_popular AND active;
