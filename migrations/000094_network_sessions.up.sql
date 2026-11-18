-- Agent-confirmed presence and asynchronous administrator disconnect commands.
CREATE TABLE network_sessions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 agent_id text NOT NULL REFERENCES network_agent_health(id) ON DELETE RESTRICT,
 agent_session_id text NOT NULL CHECK (length(agent_session_id) BETWEEN 1 AND 256),
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 subscription_id uuid NOT NULL REFERENCES user_subscriptions(id) ON DELETE CASCADE,
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
 CONSTRAINT network_sessions_identity_unique UNIQUE(agent_id,agent_session_id),
 CONSTRAINT network_sessions_activity_order CHECK (last_activity_at >= connected_at),
 CONSTRAINT network_sessions_sni_tcp CHECK (mode <> 'sni' OR protocol='tcp'),
 CONSTRAINT network_sessions_disconnected_timestamp CHECK (state <> 'disconnected' OR disconnected_at IS NOT NULL)
);
CREATE INDEX idx_network_sessions_user_seen ON network_sessions(user_id,last_report_at DESC);
CREATE INDEX idx_network_sessions_online_seen ON network_sessions(last_report_at DESC) WHERE state='online';
CREATE INDEX idx_network_sessions_agent_seen ON network_sessions(agent_id,last_report_at DESC);

CREATE TABLE network_disconnect_commands (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 network_session_id uuid NOT NULL REFERENCES network_sessions(id) ON DELETE RESTRICT,
 agent_id text NOT NULL REFERENCES network_agent_health(id) ON DELETE RESTRICT,
 actor_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 reason text NOT NULL CHECK (length(reason) BETWEEN 1 AND 1000),
 idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 128),
 state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','succeeded','failed','expired')),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 acknowledged_at timestamptz,
 error_code text,
 CONSTRAINT network_disconnect_idempotency_unique UNIQUE(actor_user_id,idempotency_key)
);
CREATE INDEX idx_network_disconnect_pending ON network_disconnect_commands(agent_id,created_at) WHERE state='pending';
CREATE INDEX idx_network_disconnect_session ON network_disconnect_commands(network_session_id,created_at DESC);

-- Agent-specific credentials are provisioned by an administrator, never by
-- self-registration. Store only a SHA-256 digest of a random 32+ byte secret.
CREATE TABLE network_session_agent_credentials (
 agent_id text PRIMARY KEY REFERENCES network_agent_health(id) ON DELETE CASCADE,
 token_sha256 bytea NOT NULL CHECK (octet_length(token_sha256)=32),
 enabled boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(),
 rotated_at timestamptz
);
