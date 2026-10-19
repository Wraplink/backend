CREATE TABLE payment_review_admins (user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE, granted_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE payment_review_events (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), receipt_id UUID NOT NULL REFERENCES card_transfer_receipts(id), reviewer_id UUID NOT NULL REFERENCES users(id), decision TEXT NOT NULL CHECK (decision IN ('approved','rejected')), created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE INDEX idx_payment_review_events_receipt ON payment_review_events(receipt_id,created_at DESC);
