CREATE TABLE payment_cards (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 card_number varchar(19) NOT NULL UNIQUE CHECK (card_number ~ '^[0-9]{16}$'),
 cardholder_name varchar(150) NOT NULL CHECK(length(trim(cardholder_name))>0),
 bank_name varchar(120) NOT NULL CHECK(length(trim(bank_name))>0),
 active boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE card_transfer_receipts ADD COLUMN payment_card_id uuid REFERENCES payment_cards(id) ON DELETE RESTRICT;
CREATE INDEX idx_payment_cards_active ON payment_cards(active,created_at);
