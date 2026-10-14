CREATE TABLE user_subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id UUID NOT NULL
        REFERENCES users(id) ON DELETE CASCADE,

    plan_id UUID NOT NULL
        REFERENCES plans(id) ON DELETE RESTRICT,

    -- Snapshot plan terms at purchase time. Later changes to the plan catalog
    -- must not change an existing customer's purchased entitlement.
    plan_code TEXT NOT NULL,
    plan_name TEXT NOT NULL,
    price_minor BIGINT NOT NULL CHECK (price_minor >= 0),
    currency VARCHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    duration_days INTEGER NOT NULL CHECK (duration_days > 0),
    data_limit_bytes BIGINT NOT NULL CHECK (data_limit_bytes > 0),
    used_bytes BIGINT NOT NULL DEFAULT 0 CHECK (used_bytes >= 0),

    status VARCHAR(30) NOT NULL DEFAULT 'pending_payment'
        CHECK (status IN ('pending_payment', 'active', 'expired', 'cancelled')),

    starts_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    activated_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CHECK (expires_at IS NULL OR starts_at IS NULL OR expires_at > starts_at)
);

CREATE INDEX idx_user_subscriptions_user_created
    ON user_subscriptions(user_id, created_at DESC);

CREATE INDEX idx_user_subscriptions_user_status
    ON user_subscriptions(user_id, status);

-- A customer can have future/pending purchases, but only one active
-- entitlement at a time.
CREATE UNIQUE INDEX uq_user_subscriptions_one_active
    ON user_subscriptions(user_id)
    WHERE status = 'active';
