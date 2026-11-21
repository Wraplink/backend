-- Saved user IP addresses. Removing an address is a soft delete so audit history remains.
CREATE TABLE user_public_ips (
 id UUID PRIMARY KEY,
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 ip_address INET NOT NULL,
 label VARCHAR(100) NOT NULL DEFAULT '',
 verified_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 removed_at TIMESTAMPTZ,
 CONSTRAINT user_public_ips_ipv4_only CHECK (family(ip_address) = 4),
 CONSTRAINT user_public_ips_label_length CHECK (char_length(label) <= 100)
);
ALTER TABLE user_public_ips ADD CONSTRAINT user_public_ips_id_user_unique UNIQUE (id, user_id);
CREATE UNIQUE INDEX user_public_ips_unique_available
 ON user_public_ips (user_id, ip_address) WHERE removed_at IS NULL;
CREATE INDEX user_public_ips_by_user ON user_public_ips (user_id, created_at DESC);

-- One selected IP per subscription. Activation is handled transactionally by the backend.
CREATE TABLE subscription_public_ip_selections (
 subscription_id UUID PRIMARY KEY REFERENCES user_subscriptions(id) ON DELETE CASCADE,
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 public_ip_id UUID NOT NULL,
 CONSTRAINT selection_ip_owned_by_user FOREIGN KEY (public_ip_id, user_id) REFERENCES user_public_ips(id, user_id),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS user_subscriptions_id_user_unique ON user_subscriptions (id, user_id);
ALTER TABLE subscription_public_ip_selections ADD CONSTRAINT selection_subscription_owned_by_user FOREIGN KEY (subscription_id, user_id) REFERENCES user_subscriptions(id, user_id);
CREATE INDEX subscription_public_ip_selections_by_ip
 ON subscription_public_ip_selections (public_ip_id);

-- Historical events must survive soft deletion of the saved address.
CREATE TABLE user_public_ip_events (
 id UUID PRIMARY KEY,
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 public_ip_id UUID REFERENCES user_public_ips(id) ON DELETE SET NULL,
 ip_address INET NOT NULL,
 event_type VARCHAR(32) NOT NULL CHECK (event_type IN ('added','verified','selected','activated','deactivated','removed','renamed')),
 subscription_id UUID,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX user_public_ip_events_by_user ON user_public_ip_events (user_id, created_at DESC);
