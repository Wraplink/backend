ALTER TABLE user_subscriptions DROP CONSTRAINT IF EXISTS user_subscriptions_unlimited_bandwidth_consistency;
ALTER TABLE plans DROP CONSTRAINT IF EXISTS plans_unlimited_bandwidth_consistency;
ALTER TABLE user_subscriptions DROP COLUMN IF EXISTS unlimited_bandwidth;
ALTER TABLE plans DROP COLUMN IF EXISTS unlimited_bandwidth;
