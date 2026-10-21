-- Backend-owned edge route public-IP inventory and durable assignments.
CREATE TABLE IF NOT EXISTS network_route_agent_ips (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  agent_id text NOT NULL,
  public_ip inet NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  last_seen_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (public_ip),
  CHECK (family(public_ip) = 4)
);
CREATE INDEX IF NOT EXISTS network_route_agent_ips_agent_idx ON network_route_agent_ips(agent_id);

CREATE TABLE IF NOT EXISTS network_route_assignments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  domain text NOT NULL,
  agent_ip_id uuid NOT NULL REFERENCES network_route_agent_ips(id),
  destination_ip inet NOT NULL,
  protocol text NOT NULL CHECK (protocol IN ('tcp','udp','tcp_udp')),
  destination_port_start integer,
  destination_port_end integer,
  state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','provisioning','active','releasing')),
  generation bigint NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (domain),
  UNIQUE (agent_ip_id),
  CHECK (family(destination_ip) = 4),
  CHECK ((destination_port_start IS NULL AND destination_port_end IS NULL)
     OR (destination_port_start BETWEEN 1 AND 65535 AND destination_port_end BETWEEN destination_port_start AND 65535))
);
CREATE INDEX IF NOT EXISTS network_route_assignments_state_idx ON network_route_assignments(state);
