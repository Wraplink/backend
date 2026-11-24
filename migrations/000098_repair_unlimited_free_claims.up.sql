-- The claim paths also rely on plan publication scheduling.
ALTER TABLE plans ADD COLUMN IF NOT EXISTS publish_at timestamptz;

-- Repair free claims created without the unlimited bandwidth snapshot.
WITH affected AS (
    SELECT s.id, s.user_id, s.status, s.starts_at
    FROM user_subscriptions AS s
    JOIN free_plan_claims AS c ON c.subscription_id = s.id AND c.user_id = s.user_id
    JOIN plans AS p ON p.id = c.plan_id AND p.id = s.plan_id
    WHERE c.state = 'active'
      AND p.unlimited_bandwidth
      AND s.price_minor = 0
      AND s.data_limit_bytes = 0
      AND NOT s.unlimited_bandwidth
      AND s.starts_at <= now()
      AND s.expires_at > now()
      AND s.status IN ('active', 'expired')
), recoverable AS (
    -- Reconciliation may already have retired the claim. Restore the most
    -- recent one only if the account has no active replacement.
    SELECT DISTINCT ON (a.user_id) a.id
    FROM affected AS a
    WHERE a.status = 'expired'
      AND NOT EXISTS (
          SELECT 1 FROM user_subscriptions AS replacement
          WHERE replacement.user_id = a.user_id AND replacement.status = 'active'
      )
    ORDER BY a.user_id, a.starts_at DESC, a.id
)
UPDATE user_subscriptions AS s
SET unlimited_bandwidth = true,
    status = CASE WHEN r.id IS NOT NULL THEN 'active' ELSE s.status END,
    updated_at = now()
FROM affected AS a
LEFT JOIN recoverable AS r ON r.id = a.id
WHERE s.id = a.id;
