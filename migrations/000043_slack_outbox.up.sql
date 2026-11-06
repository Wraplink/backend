CREATE TABLE slack_outbox (
 id bigserial PRIMARY KEY,
 event_key text NOT NULL UNIQUE,
 event_type text NOT NULL,
 payload jsonb NOT NULL,
 attempts integer NOT NULL DEFAULT 0,
 next_attempt_at timestamptz NOT NULL DEFAULT now(),
 delivered_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX slack_outbox_pending_idx ON slack_outbox(next_attempt_at, id) WHERE delivered_at IS NULL;
