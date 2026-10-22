# Administrator domain resolution: end-to-end smoke test

## Prerequisites

1. Apply backend database migrations through `000035_route_assignment_revision.up.sql` using the project's migration workflow.
2. Set the **same** 32+ character `NETWORK_CONTROL_SYNC_KEY` in backend and DNS Control configuration.
3. Configure DNS Control `backend.url` and `backend.control_key`, then restart DNS Control. It polls `GET /api/v1/internal/network/domains` periodically.
4. Log into the website with an access-administrator account and open **Admin → Network**.
5. Configure DNS Control's `dns.listen` on a reachable address. The managed DNS handler may require an active client entitlement; test from an entitled client or verify the policy snapshot separately.
6. Ensure the DNS Control node can reach the backend URL from its container/network.

## First test: deterministic specific-IP DNS answer

In **Admin → Network → Domain resolution policies**, enter:
- Domain: `resolution-test.example.com`
- Mode: **Specific IP**
- DNS response IP: `192.0.2.55` (documentation-only address; not a real production destination)

Save. The UI should show the policy and an incremented revision.

Verify the authenticated backend snapshot (replace values):

```bash
curl -fsS -H "X-Network-Control-Key: $NETWORK_CONTROL_SYNC_KEY" \
  "$BACKEND_URL/api/v1/internal/network/domains"
```

After the DNS Control refresh interval, from an **entitled client**, query:

```bash
dig @"$DNS_CONTROL_IP" resolution-test.example.com A +short
```

Expected: `192.0.2.55`. For AAAA, the current implementation returns NOERROR with an empty answer because the configured IP is IPv4.

Edit the policy to `192.0.2.56`, save, wait for refresh, and repeat `dig`; the new address should appear. Delete the policy and confirm the backend snapshot no longer includes it. Subsequent queries should follow normal upstream DNS (not the deleted static IP).

## Other modes

- **Normal:** Queries go to local DNS upstreams, or to the configured parent DNS Control when all local upstreams fail.
- **SNI hijack:** Requires an Edge Agent to report an active lease before DNS returns the agent IP; otherwise managed A queries fail closed (SERVFAIL).
- **Full route:** Requires a valid IPv4 destination, TCP/UDP protocol, an Edge Agent, and an active route lease. Do not expect a routed answer until the dataplane reports readiness.

## Troubleshooting

- HTTP 401 on snapshot: verify the shared control key.
- HTTP 403 on Admin UI: access-administrator permission is required.
- HTTP 500 after deployment: verify migrations `000032`–`000035` have been applied.
- DNS REFUSED: check the client's entitlement and DNS access rules.
- DNS SERVFAIL for hijack/route: check pending leases and Edge Agent readiness; the resolver intentionally does not expose the origin IP.
- A stale answer: wait for DNS TTL and backend synchronization interval, then query directly with `dig @...` to bypass OS resolver caches.

**Production caveat:** Domain-policy synchronization is currently polling-based. Edge Agent dataplane revocation and shared-SNI/dedicated-route address allocation require additional integration before using managed forwarding policies in production.

## Assignment revision smoke test

After applying migration `000035_route_assignment_revision.up.sql`, read the current revision:

```bash
psql "$DATABASE_URL" -Atc "SELECT revision FROM network_domain_policy_revision WHERE singleton=TRUE"
```

Create or modify a Full Route policy and ensure a healthy agent advertises a public IPv4 address. Reconcile the route assignment using the backend's normal workflow, then read the revision again. It must increase when `network_route_assignments` is inserted, updated, or deleted, even if `network_domain_policies` did not change. Verify the authenticated snapshot includes the effective assignment and generation; then verify DNS Control has consumed the new snapshot before expecting an active route DNS answer.

If the route has no valid destination or no available agent IP, the snapshot may contain an unassigned route; DNS Control must not publish a route A record until the matching agent acknowledges its current lease generation. Never interpret a successful snapshot fetch as proof of dataplane readiness.
