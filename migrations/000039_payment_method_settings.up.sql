CREATE TABLE payment_method_settings (method text PRIMARY KEY CHECK (method IN ('card','isp','crypto')), enabled boolean NOT NULL DEFAULT false, updated_at timestamptz NOT NULL DEFAULT now());
INSERT INTO payment_method_settings(method,enabled) VALUES ('card',true),('isp',false),('crypto',false);
