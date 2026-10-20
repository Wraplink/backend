DROP INDEX IF EXISTS uq_user_subscriptions_order_number;
ALTER TABLE user_subscriptions DROP COLUMN IF EXISTS order_number;
DROP SEQUENCE IF EXISTS wraplink_order_number_seq;
