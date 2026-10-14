CREATE TABLE IF NOT EXISTS plans (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 code text NOT NULL UNIQUE,
 name text NOT NULL,
 description text NOT NULL DEFAULT '',
 price_minor bigint NOT NULL CHECK (price_minor >= 0),
 currency varchar(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
 duration_days integer NOT NULL CHECK (duration_days > 0),
 data_limit_bytes bigint NOT NULL CHECK (data_limit_bytes > 0),
 active boolean NOT NULL DEFAULT true,
 sort_order integer NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CONSTRAINT plans_code_format CHECK (code ~ '^[a-z0-9][a-z0-9_-]*$')
);
CREATE INDEX IF NOT EXISTS idx_plans_active_sort ON plans (sort_order,price_minor,code) WHERE active;
