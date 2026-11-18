# Online network sessions contract (draft)

Tracks #108. Contract only; no runtime implementation yet.

## Session identity
An online network session is an edge-agent-observed and individually enforceable connection, not a web login, DNS query or subscription. Stable agent-scoped session IDs must never be recycled for new connections. Heartbeat expiry produces `unknown`, not `disconnected`.

## Proposed agent API
`POST /api/v1/agents/me/network-sessions/report`, authenticated using existing agent credentials. The backend derives agent identity from credentials. Each report contains a unique reportId, observedAt and bounded sessions array. Each session includes sessionId, userId, subscriptionId, clientIp, mode (sni/route), protocol (tcp/udp), connectedAt, lastActivityAt, uploadBytes and downloadBytes. Deduplicate report IDs, validate subscription ownership, timestamps and monotonic counters; reports never grant entitlements.

## Proposed admin APIs
`GET /api/v1/admin/network/sessions?status=online&limit=50&cursor=...` returns paginated sessions and a cursor. Summary counts distinguish unique users from connections.

`POST /api/v1/admin/network/sessions/{id}/disconnect` accepts a reason and idempotencyKey, returns HTTP 202 with commandId and `pending` status. A separate command-status read endpoint must report `pending|succeeded|failed|expired`. Only agent acknowledgement after actual termination can mark success.

## Enforcement
SNI: close exact session sockets and block immediate reconnection while revoked. Route: IP-only DNAT is not enough to identify individual users sharing a source IP; reject unsupported per-user disconnect instead of dropping unrelated traffic. UDP: terminate matching flow state. Offline agents: command remains pending or expires, never claims success. Disconnect must not alter subscriptions or web sessions.

## Security and verification
Admin-only authorization, audit actor/target/reason/outcome, bounded retention and rate limiting. Test stale reports, duplicate reports, concurrent sessions, agent outage, idempotency, SNI closure, route isolation, UDP flows, reconnection, and default /fa /en admin routes.
