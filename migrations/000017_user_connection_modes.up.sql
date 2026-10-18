-- Mode is a per-user preference, not a global DNS switch.
-- It persists while disconnected and is joined into live entitlement snapshots.
CREATE TABLE user_connection_modes (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    mode VARCHAR(16) NOT NULL DEFAULT 'smart'
        CHECK (mode IN ('smart', 'gaming', 'streaming')),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
