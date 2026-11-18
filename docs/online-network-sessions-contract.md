# Online network sessions — implementation and release gate

## Current implementation (PR #109)

The backend includes persistent network-session records, agent report and disconnect-command handlers, admin session endpoints, and database migration `000094_network_sessions`. The agent handlers are **not mounted** in `internal/httpserver/server.go` pending secure agent identity provisioning. Consequently, these APIs are **not usable in production**. The Edge Agent PR #66 contains SNI socket tracking and a separately configurable command polling client, but the client is not wired into agent startup and no authenticated session reporting exists.

Do not interpret passing CI or a mergeable PR as proof of an end-to-end Online Users feature.

## Identity and authorization blocker

The current `AuthenticateAgent` implementation resolves `X-Network-Agent-ID` through `edge_nodes.code` and requires a corresponding `network_agent_health.id` and separately provisioned `network_session_agent_credentials` token hash. SNI-only agents may have a health record without an `edge_nodes` row. Such agents cannot authenticate. Do **not** weaken this check or reuse a fleet-wide registration token.

Before enabling agent routes:
1. Define a canonical, persistent identity for **both** SNI-only and route-capable agents, with a safe migration/backfill strategy.
2. Provision and rotate a unique per-agent credential bound to that identity; store only a hash and never return secrets in logs.
3. Bind every reported session to the authenticated agent, and derive or validate user/subscription ownership from backend entitlements. An agent-reported UUID must not grant access or become trusted attribution by itself.
4. Align the backend and Edge Agent endpoint paths and JSON acknowledgement schema, and add negative authentication and cross-agent command tests.

## Verified existing identity model (migration 000015 and 000046)

The existing registration endpoint `internal/dnsregistry/agents.go` writes the
agent-provided `agent_id` into `network_agent_health.id` (TEXT primary key).
It does **not** insert an `edge_nodes` row. Migration 000015 creates
`edge_nodes.id` (UUID) and `edge_nodes.code` (unique TEXT), independently.
Therefore a fresh SNI-only agent can register and heartbeat successfully
without ever acquiring the UUID required by migration 000094. This is a
confirmed schema mismatch, not merely a hypothetical offline-agent case.

**Chosen compatibility direction for the next implementation:** retain
`edge_nodes` and its UUID foreign keys for route assignments; use the stable
registration ID from `network_agent_health.id` as the common session-control
identity. Session-control credentials must be separately provisioned against
that registered ID, never minted from the shared `X-Network-Control-Key`.
Require a live health record plus a matching enabled credential and a
constant-time digest check. Any route session must additionally resolve and
validate the matching `edge_nodes.code` assignment; SNI sessions must not
require a route node. Do not automatically create route-capable `edge_nodes`
records from untrusted registration requests.

Migration 000094 and repository methods **have not yet been refactored** to
this model. They still reference `edge_node_id UUID`, so agent endpoints must
remain unmounted until schema, SQL, authorization and tests change together.

## Network session semantics

An online network session is an agent-observed, individually enforceable connection, **not** a web login, DNS query, or subscription. Agent-local session IDs must not be recycled for new connections. Stale heartbeats mean `unknown`, not `disconnected`. Distinguish unique users from connection counts; a raw client IP does not reliably establish a user identity behind NAT.

## Intended APIs

Agent endpoints currently implemented in handler code, but **disabled in router**:
- `POST /api/v1/internal/network/sessions/report`
- `GET /api/v1/internal/network/sessions/commands`
- `POST /api/v1/internal/network/sessions/commands/{commandID}/ack`

The exact router paths must be confirmed when mounted. The agent client currently polls the commands endpoint and posts acknowledgements; it does not report sessions.

Admin contract:
- `GET /api/v1/admin/network/sessions`: paginated observed sessions with status.
- `POST /api/v1/admin/network/sessions/{id}/disconnect`: create an auditable, idempotent disconnect command and return pending status.
- Command status must distinguish `pending`, `succeeded`, `failed`, and `expired`. Never report success until the agent confirms termination.

## Enforcement and rollout

SNI: close only the identified socket. Preventing immediate reconnection requires a separate entitlement or deny policy; socket closure alone does not provide it.

Route: IP-only DNAT cannot identify individual users sharing a source IP. Reject unsupported per-user disconnect rather than interrupt unrelated traffic. UDP needs flow-specific termination. A disconnected/offline agent must not produce a false success.

Before production release, verify agent credential provisioning, registration compatibility, session lifecycle reporting, command delivery and acknowledgement, ownership validation, expiry and retries, admin authorization/auditing, default Persian plus `/fa` and `/en` routes, and a complete live end-to-end test. Deploy behind an explicit feature gate; do not expose unprovisioned endpoints.
