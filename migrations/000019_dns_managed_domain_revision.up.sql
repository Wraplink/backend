CREATE TABLE IF NOT EXISTS dns_managed_domain_revision (singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton), revision BIGINT NOT NULL DEFAULT 0);
INSERT INTO dns_managed_domain_revision(singleton,revision) VALUES(TRUE,0) ON CONFLICT(singleton) DO NOTHING;
CREATE OR REPLACE FUNCTION bump_dns_managed_domain_revision() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN UPDATE dns_managed_domain_revision SET revision=revision+1 WHERE singleton=TRUE; RETURN NULL; END; $$;
CREATE TRIGGER dns_managed_domain_revision_trigger AFTER INSERT OR UPDATE OR DELETE ON dns_managed_domains FOR EACH STATEMENT EXECUTE FUNCTION bump_dns_managed_domain_revision();
