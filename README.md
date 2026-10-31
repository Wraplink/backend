## Bootstrap administrator

Set both `WRAPLINK_BOOTSTRAP_ADMIN_EMAIL` and `WRAPLINK_BOOTSTRAP_ADMIN_PASSWORD` in the backend container environment before first startup. The password must be at least 16 characters. Do not commit these secrets to Git.

At startup, the backend creates the administrator when the email does not exist and grants the existing `access_admins`, `payment_review_admins`, and `discount_admins` privileges. If the email already belongs to a non-administrator account, startup fails rather than promoting it.

For an existing administrator with the configured email, changing the environment password and **recreating the backend container** updates the Argon2id password hash and revokes existing sessions. Unchanged passwords do not trigger a rewrite. The configured environment password remains authoritative on each restart.

```sh
docker compose -f compose.yml -f compose.prod.yml up -d --build --force-recreate backend
docker compose -f compose.yml -f compose.prod.yml logs --tail=100 backend
```

Production deployments must also set `security.secure-cookies: true` in the active configuration. Environment variables in `.env` are not automatically passed into containers unless referenced by Compose `environment:` or `env_file:`.

## Required startup data
After PostgreSQL is connected, the backend runs an idempotent reference-data bootstrap before starting the HTTP server.

Missing default plans are inserted automatically. Existing plans with the same `code` are never overwritten, so later pricing/quota changes survive application restarts.

| Code | Name | Price | Currency | Duration | Traffic |
| --- | --- | ---: | --- | ---: | ---: |
| `starter` | Starter | 1,000,000 | IRR | 30 days | 50 GiB |
| `gamer` | Gamer | 2,500,000 | IRR | 30 days | 150 GiB |
| `pro` | Pro | 5,000,000 | IRR | 30 days | 500 GiB |

The bootstrap is intentionally limited to system/reference data. It does not create users, wallet balances, payments, or subscriptions.

## Development wallet deposits
Production wallet credit must come from a trusted payment-provider settlement flow. For local development only, the backend can expose a protected settlement endpoint.

Generate a development settlement key:

```shell
export PAYMENTS_DEV_SETTLEMENT_KEY="$(openssl rand -hex 32)"
```

When `app.environment` is not `production` and this variable is set, the route below is registered:

```text
POST /api/v1/dev/deposits/{depositID}/settle
X-Settlement-Key: <PAYMENTS_DEV_SETTLEMENT_KEY>
```

Create a deposit first with the authenticated API:

```text
POST /api/v1/me/deposits
Idempotency-Key: <unique-key>

{
  "amountMinor": 5000000,
  "currency": "IRR"
}
```

Then settle it in development:

```text
POST /api/v1/dev/deposits/{depositID}/settle
X-Settlement-Key: <PAYMENTS_DEV_SETTLEMENT_KEY>

{
  "providerReference": "dev-reference-001"
}
```

Settlement is transactional and idempotent. A successful settlement creates a gateway payment record, credits the wallet, writes a wallet ledger entry, and marks the deposit succeeded. Replaying the same settlement does not credit the wallet again.

`PAYMENTS_DEV_SETTLEMENT_KEY` must be at least 32 characters when configured and must be empty in production.

## Network access and traffic usage
Connection details are environment-specific and are configured under `network` in `config.yml`. The authenticated access endpoint returns those details only when the user has a current active subscription.

```text
GET /api/v1/me/network/access
GET /api/v1/me/usage?days=30
```

Access status is derived independently from the subscription lifecycle and can be `active`, `quota_exhausted`, or `expired`.

Trusted edge/master services can report traffic deltas through an internal idempotent endpoint. Enable it with a secret of at least 32 characters:

```shell
export NETWORK_USAGE_INGEST_KEY="$(openssl rand -hex 32)"
```

```text
POST /api/v1/internal/usage/reports
X-Usage-Key: <NETWORK_USAGE_INGEST_KEY>
Content-Type: application/json

{
  "reports": [
    {
      "reportId": "edge-eu-1:counter-123:2026-10-14T12:00:00Z",
      "subscriptionId": "<subscription-uuid>",
      "source": "edge-eu-1",
      "uploadBytes": 1048576,
      "downloadBytes": 5242880,
      "reportedAt": "2026-10-14T12:00:00Z"
    }
  ]
}
```

A report ID can be retried safely. The same report ID with the same subscription/source/byte counters is returned as already applied; reusing it with different usage data is rejected. Usage reports are preserved in `traffic_usage_reports`, while `user_subscriptions.used_bytes` remains the fast aggregate used for quota enforcement.


## Per-user connection mode (Smart / Gaming / Streaming)

Apply migration **000017_user_connection_modes** before deploying the backend:

```sh
set -a
source .env.local
set +a
migrate -path migrations -database "$DATABASE_URL" up
```

The authenticated user can select a persistent mode even while disconnected:

```http
GET /api/v1/me/network/mode
Authorization: Bearer <access-token>
```

Returns `{"mode":"smart"}` (default), `{"mode":"gaming"}`, or `{"mode":"streaming"}`.

```http
PUT /api/v1/me/network/mode
Authorization: Bearer <access-token>
Content-Type: application/json

{"mode":"gaming"}
```

Invalid modes return HTTP 400; a valid PUT returns the saved mode. The choice is per user, not a global DNS server mode. The internal entitlement snapshot contains `connectionMode` for each active client IP so dns-control can apply tagged managed domain rules. DNS-control's refresh interval determines propagation delay (normally about 10 seconds). Mode changes do **not** alter the allocated DNS IP addresses, accelerate traffic, or guarantee a specific bypass without suitable provisioned DNS/edge rules.

Deploy in order: apply DB migration, deploy backend, deploy dns-control rule support, then deploy web. Add `connection_modes: ["gaming"]` or `["streaming"]` to eligible managed domain entries in dns-control's configuration; untagged rules remain available in all modes and Smart retains all managed rules.
