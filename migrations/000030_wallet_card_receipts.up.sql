CREATE TABLE wallet_card_receipts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 deposit_id uuid NOT NULL UNIQUE REFERENCES wallet_deposits(id) ON DELETE CASCADE,
 user_id uuid NOT NULL REFERENCES users(id),
 payment_card_id uuid NOT NULL REFERENCES payment_cards(id),
 filename text NOT NULL,
 content_type text NOT NULL,
 receipt_bytes bytea NOT NULL,
 status text NOT NULL DEFAULT 'pending_review' CHECK(status IN ('pending_review','approved','rejected')),
 reviewer_id uuid REFERENCES users(id),
 reviewed_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX wallet_card_receipts_pending_idx ON wallet_card_receipts(created_at DESC) WHERE status='pending_review';
