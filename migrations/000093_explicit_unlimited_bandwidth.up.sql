-- Preserve the meaning of existing zero-limit plans and subscriptions during rollout.
-- A later application change must use unlimited_bandwidth rather than infer from zero.
ALTER TABLE plans ADD COLUMN unlimited_bandwidth boolean NOT NULL DEFAULT false;
ALTER TABLE user_subscriptions ADD COLUMN unlimited_bandwidth boolean NOT NULL DEFAULT false;

UPDATE plans SET unlimited_bandwidth = true WHERE data_limit_bytes = 0;
UPDATE user_subscriptions SET unlimited_bandwidth = true WHERE data_limit_bytes = 0;

ALTER TABLE plans ADD CONSTRAINT plans_unlimited_bandwidth_consistency
  CHECK (NOT unlimited_bandwidth OR data_limit_bytes = 0);
ALTER TABLE user_subscriptions ADD CONSTRAINT user_subscriptions_unlimited_bandwidth_consistency
  CHECK (NOT unlimited_bandwidth OR data_limit_bytes = 0);

COMMENT ON COLUMN plans.unlimited_bandwidth IS 'Explicit unlimited traffic entitlement; zero bytes alone is not unlimited';
COMMENT ON COLUMN user_subscriptions.unlimited_bandwidth IS 'Subscription snapshot of unlimited traffic entitlement';
