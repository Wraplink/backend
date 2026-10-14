CREATE TABLE wallet_deposits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL
        REFERENCES users(id) ON DELETE CASCADE,
    amount_minor BIGINT NOT NULL
        CHECK (amount_minor > 0),
    currency VARCHAR(3) NOT NULL
        CHECK (currency ~ '^[A-Z]{3}$'),
    provider VARCHAR(40) NOT NULL,
    status VARCHAR(30) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'succeeded', 'failed', 'cancelled')),
    idempotency_key VARCHAR(100) NOT NULL,
    provider_reference TEXT,
    payment_id UUID
        REFERENCES payments(id) ON DELETE SET NULL,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, idempotency_key)
);

CREATE INDEX idx_wallet_deposits_user_created
    ON wallet_deposits(user_id, created_at DESC);

CREATE INDEX idx_wallet_deposits_user_status
    ON wallet_deposits(user_id, status);

CREATE UNIQUE INDEX uq_wallet_deposits_provider_reference
    ON wallet_deposits(provider, provider_reference)
    WHERE provider_reference IS NOT NULL;
