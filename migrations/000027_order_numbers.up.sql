CREATE SEQUENCE IF NOT EXISTS wraplink_order_number_seq START WITH 100001;
ALTER TABLE user_subscriptions ADD COLUMN order_number bigint;
UPDATE user_subscriptions SET order_number=nextval('wraplink_order_number_seq') WHERE order_number IS NULL;
ALTER TABLE user_subscriptions ALTER COLUMN order_number SET DEFAULT nextval('wraplink_order_number_seq');
ALTER TABLE user_subscriptions ALTER COLUMN order_number SET NOT NULL;
CREATE UNIQUE INDEX uq_user_subscriptions_order_number ON user_subscriptions(order_number);
