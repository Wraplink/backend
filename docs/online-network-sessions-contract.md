# Online network sessions — implementation and release gate

## Status (Backend PR #109 / Edge Agent PR #66)

**Not production-ready.** Keep `WRAPLINK_NETWORK_SESSIONS_ENABLED=false`.
Backend PR #109 owns persistence, agent authentication, ownership validation,
admin APIs and disconnect commands. Edge Agent PR #66 is developed separately;
do not treat the two PRs as an integrated or tested release.

Migration `000094_network_sessions` now stores `agent_id TEXT` and references
`network_agent_health(id)`. The authenticated agent identity is the stable
registered health ID, **not** `edge_nodes.id`. Route-capable agents additionally
require a valid `edge_nodes` assignment for route session reporting.

## Authentication and authorization

- Agent endpoints require `X-Network-Agent-ID` and a unique per-agent
  `Authorization: Bearer` token.
- Backend verifies an enabled `network_session_agent_credentials` entry,
  SHA-256 token digest (constant-time comparison), and a recent online
  `network_agent_health` record.
- The shared network-control/registration key is **not** a session-control token.
- Provision and rotate credentials explicitly. Never log plaintext tokens.
- SNI-only agents do not require an `edge_nodes` row. Route sessions require
  a matching active assignment.
- Admin endpoints must remain inside the existing authenticated admin router.

## Session reporting

`POST /api/v1/internal/network/sessions/report` accepts 1–100 observations,
with fields `sessionId`, `clientIp`, `mode`, `protocol`,
`connectedAt`, `lastActivityAt`, `uploadBytes`, and `downloadBytes`.

SNI reports **must omit** `userId` and `subscriptionId`. Backend resolves
both from active entitlements and active eligible subscriptions for the client
IP; zero or multiple distinct matching subscriptions cause rejection. This
fails closed for shared-IP/NAT ambiguity. Route reports currently require
identifiers and independently validate the matching active route assignment.
A raw IP alone is not universally sufficient to identify a user.

Reports are observations of individual agent-local connections, not web logins
or DNS queries. Session IDs must not be reused. Reported counters must be
nonnegative and monotonic; timestamps must be valid and ordered. Backend validates the complete payload before opening a database transaction.
Ownership lookups and session upserts then run in the same PostgreSQL
transaction; a rejected observation rolls back all writes from that batch.
Database integration tests must still verify rollback and concurrency behavior.

Edge Agent PR #66 contains an opt-in authenticated polling client and periodic
active SNI session snapshot reporting. Lifecycle end events, durable delivery,
counter reconciliation, and end-to-end testing remain outstanding.

## Disconnect commands

- `GET /api/v1/internal/network/sessions/commands` retrieves commands scoped
  to the authenticated agent.
- `POST /api/v1/internal/network/sessions/commands/{commandID}/ack`
  acknowledges `succeeded` or `failed`.
- Admin `POST /api/v1/admin/network/sessions/{id}/disconnect` creates an
  auditable, idempotent command; command status distinguishes `pending`,
  `succeeded`, `failed`, and `expired`.
- One pending command per network session is enforced by a partial unique index.
  Expired pending commands are cleaned up before a new submission.
- An administrator request is **not** a successful disconnect. Only the
  authenticated agent's acknowledgement can mark that exact session disconnected.
- SNI can close a specific socket. IP-only route DNAT cannot safely disconnect
  one user behind shared NAT; do not claim support for targeted route termination.

## Release checklist

1. Verify migration upgrade and rollback on an isolated database.
2. Provision unique agent credentials and verify negative auth, rotation and
   SNI-only registration compatibility.
3. Run unit, integration and concurrent-disconnect tests.
4. Verify SNI report attribution with unique, absent, and ambiguous entitlement
   ownership; verify quota and entitlement revocation.
5. Test live connect → report → admin list → targeted disconnect → agent ACK →
   persisted status, without disrupting other sessions.
6. Verify reporting failure recovery, agent restarts, expired commands, and
   late ACK handling.
7. Verify admin authorization/audit and default Persian, `/fa`, `/en` routes.
8. Only then enable the feature gate deliberately in production.

## Deployment

The platform forwards `WRAPLINK_NETWORK_SESSIONS_ENABLED` to Backend.
Default must remain `false`. Remote Edge Agents use their own
`WRAPLINK_SESSION_AGENT_TOKEN`; do not put these per-agent tokens in the
shared platform environment. Changing the backend gate requires recreating
the backend service.
