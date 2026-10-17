# Network entitlements

The backend owns billing/account entitlement state only. It does not register edge nodes, choose an edge, allocate egress IPs, or program nftables/SNI routes.

## User lifecycle

An authenticated user with an active subscription and remaining quota can bind the subscription to the public IPv4 address seen by the backend:

```text
POST /api/v1/me/network/entitlement
GET /api/v1/me/network/entitlement
DELETE /api/v1/me/network/entitlement
```

`POST` uses the trusted client address derived from the request. Repeating it with the same active subscription and IP is idempotent. Changing IP revokes the previous entitlement and creates a new active record.

Until the network protocol gains a stronger device identity, only one active account may claim a public IP at a time so DNS and traffic usage can be attributed unambiguously.

## dns-control synchronization

When `NETWORK_CONTROL_SYNC_KEY` is configured, the backend exposes a read-only snapshot:

```text
GET /api/v1/internal/network/entitlements
X-Network-Control-Key: <NETWORK_CONTROL_SYNC_KEY>
```

Only currently usable entitlements are included: the subscription must be active, started, not expired, and have remaining quota.

`dns-control` consumes this snapshot and owns DNS/routing decisions. `edge-agent` remains managed by `dns-control`.

## Migration history

Migration `000015_edge_provisioning` remains in the repository because it may already have been applied by existing databases. Its runtime implementation is superseded and removed from the backend. Migration `000016_network_entitlements` introduces the replacement account-level entitlement model.
