-- Discount codes are server-authoritative; amounts are calculated in minor currency units.
CREATE TABLE discount_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL UNIQUE CHECK (code = upper(code) AND length(code) BETWEEN 3 AND 64),
    discount_type TEXT NOT NULL CHECK (discount_type IN ('percent', 'fixed')),
    discount_value BIGINT NOT NULL CHECK (discount_value > 0),
    currency VARCHAR(3),
    active BOOLEAN NOT NULL DEFAULT true,
    starts_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    max_redemptions INTEGER CHECK (max_redemptions > 0),
    redemptions INTEGER NOT NULL DEFAULT 0 CHECK (redemptions >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (discount_type <> 'percent' OR (discount_value <= 10000 AND currency IS NULL)),
    CHECK (discount_type <> 'fixed' OR currency IS NOT NULL),
    CHECK (expires_at IS NULL OR starts_at IS NULL OR expires_at > starts_at)
);
CREATE TABLE discount_redemptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    discount_id UUID NOT NULL REFERENCES discount_codes(id),
    user_id UUID NOT NULL REFERENCES users(id),
    subscription_id UUID NOT NULL REFERENCES user_subscriptions(id),
    amount_minor BIGINT NOT NULL CHECK (amount_minor >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (discount_id, subscription_id)
);
-- Receipt content is stored in PostgreSQL rather than a publicly accessible filesystem.
CREATE TABLE card_transfer_receipts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    subscription_id UUID NOT NULL REFERENCES user_subscriptions(id),
    filename TEXT NOT NULL,
    content_type TEXT NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png', 'image/webp', 'application/pdf')),
    receipt_bytes BYTEA NOT NULL CHECK (octet_length(receipt_bytes) BETWEEN 1 AND 5242880),
    status TEXT NOT NULL DEFAULT 'pending_review' CHECK (status IN ('pending_review','approved','rejected')),
    discount_id UUID REFERENCES discount_codes(id),
    discount_minor BIGINT NOT NULL DEFAULT 0 CHECK (discount_minor >= 0),
    payable_minor BIGINT NOT NULL CHECK (payable_minor >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_at TIMESTAMPTZ,
    UNIQUE (subscription_id)
);
CREATE INDEX idx_card_transfer_receipts_user_created ON card_transfer_receipts(user_id, created_at DESC);
