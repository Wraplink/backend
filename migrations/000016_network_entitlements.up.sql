CREATE TABLE network_entitlements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subscription_id UUID NOT NULL REFERENCES user_subscriptions(id) ON DELETE CASCADE,
    client_ip INET NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'revoked')),
    activated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_network_entitlements_user_active
    ON network_entitlements(user_id)
    WHERE status = 'active';

CREATE UNIQUE INDEX uq_network_entitlements_subscription_active
    ON network_entitlements(subscription_id)
    WHERE status = 'active';

-- DNS requests are attributable by source public IP. Until the protocol gains
-- a stronger device identity, two different accounts cannot actively claim
-- the same public IP without making usage attribution ambiguous.
CREATE UNIQUE INDEX uq_network_entitlements_client_ip_active
    ON network_entitlements(client_ip)
    WHERE status = 'active';

CREATE INDEX idx_network_entitlements_status_updated
    ON network_entitlements(status, updated_at DESC);
