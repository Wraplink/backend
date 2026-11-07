-- Rollback requires no unlimited plans/subscriptions.
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM plans WHERE data_limit_bytes=0)
 OR EXISTS (SELECT 1 FROM user_subscriptions WHERE data_limit_bytes=0) THEN
 RAISE EXCEPTION 'Cannot revert unlimited plans while zero-limit records exist';
 END IF;
END $$;
ALTER TABLE plans DROP CONSTRAINT IF EXISTS plans_data_limit_bytes_check;
ALTER TABLE plans ADD CONSTRAINT plans_data_limit_bytes_check CHECK (data_limit_bytes>0);
ALTER TABLE user_subscriptions DROP CONSTRAINT IF EXISTS user_subscriptions_data_limit_bytes_check;
ALTER TABLE user_subscriptions ADD CONSTRAINT user_subscriptions_data_limit_bytes_check CHECK (data_limit_bytes>0);
