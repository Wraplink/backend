CREATE INDEX IF NOT EXISTS idx_network_entitlements_usage_attribution
    ON network_entitlements (client_ip, activated_at, revoked_at)
    INCLUDE (subscription_id)
    WHERE status IN ('active', 'revoked');
