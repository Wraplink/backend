DROP TRIGGER IF EXISTS network_domain_policy_revision_change ON network_domain_policies;
DROP FUNCTION IF EXISTS bump_network_domain_policy_revision();
DROP TABLE IF EXISTS network_domain_policies;
DROP TABLE IF EXISTS network_domain_policy_revision;
