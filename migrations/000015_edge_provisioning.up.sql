CREATE TABLE edge_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL UNIQUE CHECK (code ~ '^[a-z0-9][a-z0-9_-]*$'),
    region TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'online'
        CHECK (status IN ('online', 'offline', 'draining')),
    enabled BOOLEAN NOT NULL DEFAULT true,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE edge_public_ips (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    edge_node_id UUID NOT NULL REFERENCES edge_nodes(id) ON DELETE CASCADE,
    ip INET NOT NULL UNIQUE,
    status VARCHAR(20) NOT NULL DEFAULT 'available'
        CHECK (status IN ('available', 'allocated', 'draining')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_edge_public_ips_node_status
    ON edge_public_ips(edge_node_id, status);

CREATE TABLE network_access_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subscription_id UUID NOT NULL REFERENCES user_subscriptions(id) ON DELETE CASCADE,
    edge_node_id UUID NOT NULL REFERENCES edge_nodes(id) ON DELETE RESTRICT,
    egress_ip_id UUID NOT NULL REFERENCES edge_public_ips(id) ON DELETE RESTRICT,
    client_ip INET NOT NULL,
    state VARCHAR(20) NOT NULL DEFAULT 'provisioning'
        CHECK (state IN ('provisioning', 'active', 'suspended', 'releasing', 'released')),
    reason TEXT,
    allocated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_activity_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    released_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_network_access_subscription_live
    ON network_access_assignments(subscription_id)
    WHERE state IN ('provisioning', 'active', 'suspended', 'releasing');

CREATE UNIQUE INDEX uq_network_access_user_live
    ON network_access_assignments(user_id)
    WHERE state IN ('provisioning', 'active', 'suspended', 'releasing');

CREATE UNIQUE INDEX uq_network_access_egress_live
    ON network_access_assignments(egress_ip_id)
    WHERE state IN ('provisioning', 'active', 'suspended', 'releasing');

CREATE INDEX idx_network_access_edge_state
    ON network_access_assignments(edge_node_id, state);

CREATE INDEX idx_network_access_last_activity
    ON network_access_assignments(last_activity_at)
    WHERE state IN ('provisioning', 'active');

CREATE TABLE edge_sync_events (
    revision BIGSERIAL PRIMARY KEY,
    edge_node_id UUID NOT NULL REFERENCES edge_nodes(id) ON DELETE CASCADE,
    access_id UUID NOT NULL REFERENCES network_access_assignments(id) ON DELETE CASCADE,
    event_type VARCHAR(30) NOT NULL
        CHECK (event_type IN ('access.upsert', 'access.remove')),
    payload JSONB NOT NULL,
    acknowledged_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_edge_sync_events_node_revision
    ON edge_sync_events(edge_node_id, revision);
