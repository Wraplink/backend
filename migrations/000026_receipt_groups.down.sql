DROP INDEX IF EXISTS idx_card_transfer_receipts_payment_group;
ALTER TABLE card_transfer_receipts DROP COLUMN IF EXISTS payment_group_id;
