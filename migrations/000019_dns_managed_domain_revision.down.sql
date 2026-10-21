DROP TRIGGER IF EXISTS dns_managed_domain_revision_trigger ON dns_managed_domains;
DROP FUNCTION IF EXISTS bump_dns_managed_domain_revision();
DROP TABLE IF EXISTS dns_managed_domain_revision;
