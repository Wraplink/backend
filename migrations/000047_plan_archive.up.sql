ALTER TABLE plans ADD COLUMN archived_at timestamptz;
CREATE INDEX idx_plans_archived_at ON plans (archived_at) WHERE archived_at IS NOT NULL;
