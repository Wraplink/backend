CREATE TABLE network_domain_policies (
 domain TEXT PRIMARY KEY,
 mode TEXT NOT NULL CHECK (mode IN ('hijack','route','specific_ip','normal')),
 specific_ip INET,
 route JSONB,
 connection_modes TEXT[] NOT NULL DEFAULT '{}',
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK ((mode = 'specific_ip' AND specific_ip IS NOT NULL AND route IS NULL)
 OR (mode = 'route' AND route IS NOT NULL AND specific_ip IS NULL)
 OR (mode IN ('hijack','normal') AND route IS NULL AND specific_ip IS NULL))
);
CREATE TABLE network_domain_policy_revision (
 singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
 revision BIGINT NOT NULL DEFAULT 1,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO network_domain_policy_revision(singleton) VALUES(TRUE);
CREATE OR REPLACE FUNCTION bump_network_domain_policy_revision() RETURNS TRIGGER AS $$
BEGIN
 UPDATE network_domain_policy_revision SET revision=revision+1,updated_at=now() WHERE singleton=TRUE;
 RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER network_domain_policy_revision_change
AFTER INSERT OR UPDATE OR DELETE ON network_domain_policies
FOR EACH STATEMENT EXECUTE FUNCTION bump_network_domain_policy_revision();
