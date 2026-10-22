CREATE TABLE IF NOT EXISTS dns_managed_domains (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 domain TEXT NOT NULL UNIQUE,
 enabled BOOLEAN NOT NULL DEFAULT true,
 mode TEXT NOT NULL CHECK (mode IN ('SNI_HIJACK','FULL_ROUTE','DIRECT')),
 agent_id UUID,
 override_ip INET,
 fallback_mode TEXT NOT NULL DEFAULT 'DIRECT' CHECK (fallback_mode IN ('DIRECT','REFUSE')),
 priority INTEGER NOT NULL DEFAULT 100,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CONSTRAINT dns_domain_nonempty CHECK (length(trim(domain)) > 0)
);
CREATE INDEX IF NOT EXISTS dns_managed_domains_enabled_priority_idx ON dns_managed_domains (enabled, priority DESC);
