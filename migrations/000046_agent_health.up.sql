CREATE SEQUENCE IF NOT EXISTS network_agent_health_revision_seq START WITH 1;
CREATE TABLE IF NOT EXISTS network_agent_health (
  id text PRIMARY KEY,
  region text NOT NULL DEFAULT '',
  sni_capacity integer NOT NULL DEFAULT 0 CHECK (sni_capacity >= 0),
  route_capacity integer NOT NULL DEFAULT 0 CHECK (route_capacity >= 0),
  status text NOT NULL DEFAULT 'online' CHECK (status IN ('online','offline')),
  registered_at timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  revision bigint NOT NULL DEFAULT nextval('network_agent_health_revision_seq')
);
CREATE INDEX IF NOT EXISTS network_agent_health_last_seen_idx ON network_agent_health (last_seen_at);
