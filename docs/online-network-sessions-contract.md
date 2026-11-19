# Online network sessions — implementation and release gate

## Status (Backend PR #109 / merged Edge Agent PR #66)

**Not production-ready.** Keep `WRAPLINK_NETWORK_SESSIONS_ENABLED=false`.
Backend PR #109 owns persistence, agent authentication, ownership validation,
admin APIs and disconnect commands. Edge Agent PR #66 was merged into its
`main` branch at `88847032a2933243909851a70f348d8ddee5169c`.
The merged agent and this backend branch have not yet passed a live
end-to-end release verification.

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
PostgreSQL integration tests cover transactional rollback, concurrent pending-command uniqueness, agent authentication, SNI ownership attribution, and the authenticated disconnect acknowledgement lifecycle.

The merged Edge Agent contains an opt-in authenticated polling client and periodic
active SNI session snapshot reporting (currently every 10 seconds). Lifecycle end
events, durable delivery, counter reconciliation, and live end-to-end testing
remain outstanding. The agent requires a configured HTTPS backend URL and an
agent ID matching the registered `network_agent_health.id`.

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
- Only online SNI/TCP sessions can receive new targeted disconnect commands.
  IP-only route DNAT cannot safely disconnect one user behind shared NAT;
  Route sessions are rejected rather than queued for unsupported termination.

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

## Production preflight: agent credential provisioning

Do not enable the backend feature gate until the deployment and live checks
below are scheduled. Agent authentication depends on the exact
`network_agent_health.id`, an `online` health status, and a heartbeat within
two minutes. The Edge Agent session token is **different** from its registration
or shared network-control token.

1. Identify the registered agent ID on the backend database:

   ```sql
   SELECT id, status, last_seen_at
   FROM network_agent_health
   ORDER BY last_seen_at DESC;
   ```

2. On a trusted administrative workstation, generate a unique secret and
   calculate its SHA-256 digest. Do not paste the secret into shell history,
   tickets, GitHub Actions logs, or shared environment files. For example:

   ```bash
   umask 077
   openssl rand -hex 32 > ./agent-session-token
   sha256sum ./agent-session-token
   ```

   The first 64 hex characters of the `sha256sum` output are the digest;
   the file contains the **plaintext token** for that one agent.

3. Provision the digest on the backend database using a transaction, replacing
   the two placeholders with the registered agent ID and the **digest only**.
   Do not store plaintext in PostgreSQL:

   ```sql
   BEGIN;
   INSERT INTO network_session_agent_credentials
     (agent_id, token_sha256, enabled)
   VALUES
     ('REPLACE_AGENT_HEALTH_ID', decode('REPLACE_SHA256_HEX', 'hex'), true)
   ON CONFLICT (agent_id)
   DO UPDATE SET token_sha256 = EXCLUDED.token_sha256,
                 enabled = true,
                 rotated_at = now();
   COMMIT;
   ```

   This intentionally replaces any existing token for the same agent.
   Coordinate rotation with the Edge Agent deployment; old tokens stop
   working immediately. Never copy one agent's token to another.

4. Supply the **plaintext** token to only that agent as
   `WRAPLINK_SESSION_AGENT_TOKEN` using its protected local environment or
   secrets mechanism. Verify the agent's session reporting/polling configuration
   points to the backend's intended HTTPS API and uses its registered health
   ID as `X-Network-Agent-ID`. Restrict environment-file permissions and
   restart the agent as required.

5. Before enabling the backend feature gate, verify the credential with a
   harmless request to the session command polling endpoint in an isolated
   test deployment. Expected results: the correct token and fresh online
   heartbeat authorize the request; a wrong token, wrong agent ID, or disabled
   credential returns HTTP 401. Do not use a production disconnect command
   as a connectivity probe.

6. After approval for a controlled production verification window, enable
   `WRAPLINK_NETWORK_SESSIONS_ENABLED=true` on the backend and recreate that
   service. Confirm a new SNI connection is reported and correctly attributed;
   submit a targeted disconnect; verify the exact agent-local socket closes,
   the command becomes `succeeded`, and unrelated sessions remain online.
   Revert the flag to `false` and recreate the backend if verification fails.
   Disabling the flag stops new agent session API requests; it does not undo
   sessions or commands already persisted.

### Safe read-only diagnostics

```sql
SELECT a.id, a.status, a.last_seen_at,
       (c.agent_id IS NOT NULL) AS credential_provisioned,
       COALESCE(c.enabled, false) AS credential_enabled,
       c.rotated_at
FROM network_agent_health a
LEFT JOIN network_session_agent_credentials c ON c.agent_id = a.id
ORDER BY a.last_seen_at DESC;
```

Do not select `token_sha256` into support logs or expose tokens in admin UI.
For credential revocation, set `enabled=false` for the affected agent and
confirm its session-control API calls are denied. This does not revoke the
agent's other registration credentials.
