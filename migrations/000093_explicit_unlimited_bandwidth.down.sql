-- Restore the previous zero-limit semantics before removing the explicit flag.
CREATE OR REPLACE FUNCTION promote_user_subscription(p_user_id uuid)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE v_current uuid; v_next uuid;
BEGIN
 PERFORM 1 FROM users WHERE id=p_user_id FOR UPDATE;
 IF NOT FOUND THEN RETURN; END IF;
 UPDATE user_subscriptions SET status='expired',updated_at=now()
 WHERE user_id=p_user_id AND status='active'
 AND (expires_at<=now() OR (data_limit_bytes>0 AND used_bytes>=data_limit_bytes));
 UPDATE network_entitlements e SET status='revoked',revoked_at=now(),updated_at=now()
 WHERE e.user_id=p_user_id AND e.status='active'
 AND NOT EXISTS (SELECT 1 FROM user_subscriptions s WHERE s.id=e.subscription_id
   AND s.status='active' AND s.expires_at>now()
   AND (s.data_limit_bytes=0 OR s.used_bytes<s.data_limit_bytes));
 SELECT id INTO v_current FROM user_subscriptions
 WHERE user_id=p_user_id AND status='active' LIMIT 1;
 IF v_current IS NOT NULL THEN RETURN; END IF;
 SELECT id INTO v_next FROM user_subscriptions
 WHERE user_id=p_user_id AND status='confirmed'
 ORDER BY created_at,id LIMIT 1 FOR UPDATE;
 IF v_next IS NULL THEN RETURN; END IF;
 UPDATE user_subscriptions SET status='active',starts_at=now(),
 expires_at=now()+(duration_days*interval '1 day'),activated_at=now(),updated_at=now()
 WHERE id=v_next;
END $$;

CREATE OR REPLACE FUNCTION promote_subscription_on_usage()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.status='active' AND NEW.data_limit_bytes>0 AND NEW.used_bytes>=NEW.data_limit_bytes THEN
   PERFORM promote_user_subscription(NEW.user_id);
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS trg_promote_subscription_on_usage ON user_subscriptions;
CREATE TRIGGER trg_promote_subscription_on_usage
 AFTER UPDATE OF used_bytes ON user_subscriptions
 FOR EACH ROW WHEN (NEW.data_limit_bytes>0 AND NEW.used_bytes>=NEW.data_limit_bytes AND OLD.used_bytes<NEW.used_bytes)
 EXECUTE FUNCTION promote_subscription_on_usage();

ALTER TABLE user_subscriptions DROP CONSTRAINT IF EXISTS user_subscriptions_unlimited_bandwidth_consistency;
ALTER TABLE plans DROP CONSTRAINT IF EXISTS plans_unlimited_bandwidth_consistency;
ALTER TABLE user_subscriptions DROP COLUMN IF EXISTS unlimited_bandwidth;
ALTER TABLE plans DROP COLUMN IF EXISTS unlimited_bandwidth;
