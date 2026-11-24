CREATE TABLE network_ip_approvals (
    user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    id uuid NOT NULL UNIQUE,
    client_ip inet NOT NULL CHECK (family(client_ip) = 4 AND masklen(client_ip) = 32),
    expires_at timestamptz NOT NULL
);
