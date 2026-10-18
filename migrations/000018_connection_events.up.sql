CREATE TABLE connection_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(16) NOT NULL CHECK (type IN ('success', 'info', 'warning')),
    title VARCHAR(200) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_connection_events_user_created
    ON connection_events (user_id, created_at DESC, id DESC);

-- Preserve existing connection history when deploying this feature.
INSERT INTO connection_events (user_id, type, title, created_at)
SELECT user_id, 'success', 'Connection activated', activated_at
FROM network_entitlements;

INSERT INTO connection_events (user_id, type, title, created_at)
SELECT user_id, 'info', 'Connection disconnected', revoked_at
FROM network_entitlements
WHERE revoked_at IS NOT NULL;
