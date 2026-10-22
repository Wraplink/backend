-- Route assignment changes alter the effective DNS policy snapshot even when
-- the administrator's domain policy row itself is unchanged.
CREATE TRIGGER network_route_assignment_revision_change
AFTER INSERT OR UPDATE OR DELETE ON network_route_assignments
FOR EACH STATEMENT EXECUTE FUNCTION bump_network_domain_policy_revision();
