CREATE TABLE network_saved_ips (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_ip inet NOT NULL CHECK (family(client_ip) = 4 AND masklen(client_ip) = 32),
    last_used_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, client_ip)
);

INSERT INTO network_saved_ips (user_id, client_ip, last_used_at)
SELECT user_id, host(client_ip)::inet, max(activated_at)
FROM network_entitlements
WHERE family(client_ip) = 4
GROUP BY user_id, host(client_ip);
