# Agent registry and Full Route rollout

The Backend, DNS Control and Edge Agent changes must be deployed as a coordinated release. Passing CI is not an end-to-end failover test.

## Compatibility and rollout order

1. Back up PostgreSQL and apply Backend migration `000046_agent_health`. Keep the existing routing stack available during migration.
2. Deploy Backend with the agent registration/heartbeat endpoints. **Important:** the updated route reconciler and activation ACK path require a fresh `network_agent_health` record. If existing agents are not yet publishing Backend heartbeats, their Full Route assignments can be demoted or excluded. Do not roll out this Backend build to a production Full Route fleet without a coordinated Edge Agent rollout or a maintenance window.
3. Deploy Edge Agent with its existing Backend URL and control key configured. Confirm each agent registers successfully and continues heartbeating. Confirm its route IP inventory heartbeat also remains healthy.
4. Verify Backend agent registry freshness before relying on Full Route. Example SQL:
   ```sql
   SELECT id, region, status, sni_capacity, route_capacity,
          now() - last_seen_at AS heartbeat_age
   FROM network_agent_health
   ORDER BY id;
   ```
5. Deploy DNS Control and enable `WRAPLINK_BACKEND_AGENT_SNAPSHOT_ENABLED=true` **only after** the Backend snapshot endpoint and all required Edge Agent heartbeats are working. Confirm DNS Control can fetch every snapshot page without revision conflicts.
6. Verify a managed SNI domain and an active Full Route domain resolve to their intended live agent IPs. Verify a stale/offline agent is excluded and an ACK from an old assignment generation is rejected.

## Operational guardrails

- Backend route eligibility requires **both** a fresh route IP inventory heartbeat and a fresh Backend agent health heartbeat.
- A route assignment remains pending until its current generation is acknowledged. DNS must not serve pending or expired assignments.
- A backend snapshot revision can change while pages are being fetched. DNS Control rejects mixed-revision pages; sustained high write rates can require retry/backoff and a more robust consistent snapshot protocol.
- PostgreSQL route assignment uniqueness and in-place reassignment have not yet been validated with concurrent failover integration tests. In particular, test IP reassignment collisions and multi-domain migrations.
- A passing unit-test workflow does not prove zero-downtime failover, external DNS propagation, or packet forwarding. Test those in staging before production rollout.

## Rollback

Disable the Backend snapshot feature in DNS Control first if its snapshot feed fails. Roll back application versions as a coordinated set rather than leaving a Backend version that requires authoritative health paired with older Edge Agents that do not register. Preserve migration data during rollback unless a separate database rollback has been validated.
