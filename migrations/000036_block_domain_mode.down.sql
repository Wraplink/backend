DELETE FROM network_domain_policies WHERE mode = 'block';
DELETE FROM dns_managed_domains WHERE mode = 'BLOCK';
ALTER TABLE network_domain_policies DROP CONSTRAINT IF EXISTS network_domain_policies_mode_check;
ALTER TABLE network_domain_policies ADD CONSTRAINT network_domain_policies_mode_check
CHECK (mode IN ('hijack','route','specific_ip','normal'));
ALTER TABLE network_domain_policies DROP CONSTRAINT IF EXISTS network_domain_policies_check;
ALTER TABLE network_domain_policies ADD CONSTRAINT network_domain_policies_check
CHECK ((mode = 'specific_ip' AND specific_ip IS NOT NULL AND route IS NULL)
 OR (mode = 'route' AND route IS NOT NULL AND specific_ip IS NULL)
 OR (mode IN ('hijack','normal') AND route IS NULL AND specific_ip IS NULL));
ALTER TABLE dns_managed_domains DROP CONSTRAINT IF EXISTS dns_managed_domains_mode_check;
ALTER TABLE dns_managed_domains ADD CONSTRAINT dns_managed_domains_mode_check
CHECK (mode IN ('SNI_HIJACK','FULL_ROUTE','DIRECT'));
