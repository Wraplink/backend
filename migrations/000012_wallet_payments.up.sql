CREATE TABLE wallet_accounts (
    user_id UUID NOT NULL
        REFERENCES users(id) ON DELETE CASCADE,
    currency VARCHAR(3) NOT NULL
        CHECK (currency ~ '^[A-Z]{3}$'),
    balance_minor BIGINT NOT NULL DEFAULT 0
        CHECK (balance_minor >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, currency)
);

CREATE TABLE payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL
        REFERENCES users(id) ON DELETE CASCADE,
    subscription_id UUID
        REFERENCES user_subscriptions(id) ON DELETE SET NULL,
    method VARCHAR(30) NOT NULL
        CHECK (method IN ('wallet', 'gateway', 'card_to_card', 'crypto')),
    amount_minor BIGINT NOT NULL
        CHECK (amount_minor >= 0),
    currency VARCHAR(3) NOT NULL
        CHECK (currency ~ '^[A-Z]{3}$'),
    status VARCHAR(30) NOT NULL
        CHECK (status IN ('pending', 'succeeded', 'failed', 'cancelled', 'refunded')),
    idempotency_key VARCHAR(100) NOT NULL,
    provider_reference TEXT,
    metadata JSONB NOT NULL DEFAULT '{}',
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, idempotency_key)
);

CREATE INDEX idx_payments_user_created
    ON payments(user_id, created_at DESC);

CREATE INDEX idx_payments_subscription
    ON payments(subscription_id)
    WHERE subscription_id IS NOT NULL;

CREATE UNIQUE INDEX uq_payments_subscription_succeeded
    ON payments(subscription_id)
    WHERE subscription_id IS NOT NULL
      AND status = 'succeeded';

CREATE TABLE wallet_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    currency VARCHAR(3) NOT NULL,
    payment_id UUID
        REFERENCES payments(id) ON DELETE SET NULL,
    direction VARCHAR(10) NOT NULL
        CHECK (direction IN ('credit', 'debit')),
    reason VARCHAR(40) NOT NULL,
    amount_minor BIGINT NOT NULL
        CHECK (amount_minor > 0),
    balance_after_minor BIGINT NOT NULL
        CHECK (balance_after_minor >= 0),
    idempotency_key VARCHAR(140) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (user_id, currency)
        REFERENCES wallet_accounts(user_id, currency) ON DELETE CASCADE,
    UNIQUE (user_id, idempotency_key)
);

CREATE INDEX idx_wallet_transactions_user_currency_created
    ON wallet_transactions(user_id, currency, created_at DESC);
