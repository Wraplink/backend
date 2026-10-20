DROP TRIGGER IF EXISTS trg_promote_subscription_on_usage ON user_subscriptions;
DROP FUNCTION IF EXISTS promote_subscription_on_usage();
DROP FUNCTION IF EXISTS promote_user_subscription(uuid);
DROP INDEX IF EXISTS idx_user_subscriptions_confirmed_queue;
-- Existing confirmed subscriptions must be reconciled before reverting this migration.
DO $$ BEGIN IF EXISTS (SELECT 1 FROM user_subscriptions WHERE status='confirmed') THEN RAISE EXCEPTION 'Cannot roll back while confirmed subscriptions exist'; END IF; END $$;
ALTER TABLE user_subscriptions DROP CONSTRAINT IF EXISTS user_subscriptions_status_check;
ALTER TABLE user_subscriptions ADD CONSTRAINT user_subscriptions_status_check CHECK(status IN ('pending_payment','active','expired','cancelled'));
