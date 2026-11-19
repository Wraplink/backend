-- Draft schema for network session presence and disconnect commands.
-- This file is NOT wired into the application's migration runner.
-- Review existing migration naming and database conventions before deployment.
CREATE TABLE IF NOT EXISTS network_sessions (
  id uuid PRIMARY KEY,
  agent_id text NOT NULL REFERENCES network_agent_health(id) ON DELETE RESTRICT,
  agent_session_id text NOT NULL CHECK (length(agent_session_id) BETWEEN 1 AND 256),
  user_id uuid NOT NULL,
  subscription_id uuid NOT NULL,
  client_ip inet NOT NULL,
  mode text NOT NULL CHECK (mode IN ('sni','route')),
  protocol text NOT NULL CHECK (protocol IN ('tcp','udp')),
  connected_at timestamptz NOT NULL,
  last_activity_at timestamptz NOT NULL,
  last_report_at timestamptz NOT NULL,
  upload_bytes bigint NOT NULL DEFAULT 0 CHECK (upload_bytes >= 0),
  download_bytes bigint NOT NULL DEFAULT 0 CHECK (download_bytes >= 0),
  state text NOT NULL DEFAULT 'online' CHECK (state IN ('online','unknown','disconnected')),
  disconnected_at timestamptz,
  UNIQUE (agent_id,agent_session_id),
  CHECK (last_activity_at >= connected_at),
  CHECK (mode <> 'sni' OR protocol = 'tcp')
);
CREATE INDEX IF NOT EXISTS network_sessions_user_idx ON network_sessions (user_id,last_report_at DESC);
CREATE INDEX IF NOT EXISTS network_sessions_active_idx ON network_sessions (last_report_at DESC) WHERE state = 'online';

CREATE TABLE IF NOT EXISTS network_disconnect_commands (
  id uuid PRIMARY KEY,
  network_session_id uuid NOT NULL REFERENCES network_sessions(id),
  agent_id text NOT NULL REFERENCES network_agent_health(id) ON DELETE RESTRICT,
  actor_user_id uuid NOT NULL,
  reason text NOT NULL,
  idempotency_key text NOT NULL,
  state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','succeeded','failed','expired')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  acknowledged_at timestamptz,
  error_code text,
  UNIQUE (actor_user_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS network_disconnect_commands_pending_idx ON network_disconnect_commands (agent_id,created_at) WHERE state = 'pending';

-- Separately provisioned agent credentials; the shared registration key is not
-- an acceptable session-control credential. Never store plaintext secrets.
CREATE TABLE IF NOT EXISTS network_session_agent_credentials (
  agent_id text PRIMARY KEY REFERENCES network_agent_health(id) ON DELETE CASCADE,
  token_sha256 bytea NOT NULL CHECK (octet_length(token_sha256)=32),
  enabled boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  rotated_at timestamptz
);
