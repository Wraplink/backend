# Full Route public-IP allocation contract

Backend is authoritative for route IP inventory and assignment. A route consumes exactly one public IPv4 address on one agent, regardless of the agent's SNI capacity.

## Desired state
- Agent registration/heartbeat advertises agent ID and its usable public IPv4 inventory. The backend validates ownership and reconciles addresses. An address cannot belong to multiple agents.
- Every enabled Full Route policy is queued until an address is available. Under a transaction and row-level locks, allocate the oldest waiting domain to the first available healthy agent IP. Never allocate an IP twice.
- Assignment has generation and state: pending -> provisioning -> active. Only after edge acknowledgement may DNS Control publish the allocated public IP as an A record.
- Agent receives domain, public_ip, destination_ip, protocol, port range, assignment ID and generation. It must bind the specified IP, install TCP/UDP forwarding, and acknowledge the same generation.
- When an agent goes offline or fails its lease, withdraw its DNS answers first, invalidate its assignment generation, then reclaim its IP assignments for healthy agents. An old agent acknowledgement must never reactivate a superseded generation.
- A newly registered agent or route policy triggers reconciliation. A spare IP remains available; a route with no IP remains waiting.
- DNS Control receives an authoritative versioned assignment snapshot from backend; never independently assigns route IPs. Unassigned or unacknowledged Full Route policies return SERVFAIL, never upstream destination IP.
- Specific IP, SNI round robin and ordinary upstream DNS are separate modes and remain unchanged.

## Storage
Migration `000034` creates `network_route_agent_ips` and `network_route_assignments`. Migration `000035` bumps the DNS policy revision on assignment changes. Unique constraints ensure one assignment per domain and one per IP. Backend route reconciliation, assignment generation updates, and effective assignment snapshots have been implemented. Deployment readiness still requires end-to-end verification of agent acknowledgement, DNS publication, revocation, and failover; see [backend#34](https://github.com/Wraplink/backend/issues/34) and [dns-control#20](https://github.com/Wraplink/dns-control/issues/20).

## Security and operations
- Use authenticated control-plane endpoints for agent registration, snapshot and acknowledgement.
- Confirm the advertised IP is assigned to the edge host before publishing it.
- Configure routing/firewall rules with fail-closed defaults, source authorization and cleanup on revoke.
- Increment generation on reassignment and reject stale acknowledgements.
- Make DNS TTL short and expect cache convergence delay after failure.

## Implementation and verification tracker

- Backend domain policy administration and effective route assignment snapshots: implemented; verify against the deployed schema.
- Route inventory reconciliation and fail-closed behavior for unresolved destinations: implemented; unit regression coverage in [backend PR #31](https://github.com/Wraplink/backend/pull/31).
- Revision propagation on assignment changes: implemented by migration `000035` in [backend PR #32](https://github.com/Wraplink/backend/pull/32); validate deployment and DNS Control polling under [backend#33](https://github.com/Wraplink/backend/issues/33).
- Agent-side lease application, acknowledgement, public IP readiness, and revocation: integration validation remains under [edge-agent#13](https://github.com/Wraplink/edge-agent/issues/13).
- SNI origin resolution safety: tracked separately in [edge-agent#12](https://github.com/Wraplink/edge-agent/issues/12).
- Repeatable end-to-end DNS tests: [dns-control#20](https://github.com/Wraplink/dns-control/issues/20).

Do not mark a route active solely because it is allocated in the backend. A DNS A record for Full Route must remain unavailable until the matching current-generation agent lease is acknowledged and the dataplane is ready.
