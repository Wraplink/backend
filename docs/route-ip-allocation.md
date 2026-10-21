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
Migration 000033 creates network_route_agent_ips and network_route_assignments. Unique constraints ensure one assignment per domain and one per IP. The tables are groundwork; API handlers, reconciler, heartbeat, snapshot and web UI must be added before this feature can be deployed.

## Security and operations
- Use authenticated control-plane endpoints for agent registration, snapshot and acknowledgement.
- Confirm the advertised IP is assigned to the edge host before publishing it.
- Configure routing/firewall rules with fail-closed defaults, source authorization and cleanup on revoke.
- Increment generation on reassignment and reject stale acknowledgements.
- Make DNS TTL short and expect cache convergence delay after failure.
