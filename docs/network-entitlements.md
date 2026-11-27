# Network entitlements

The backend owns billing/account entitlement state only. It does not register edge nodes, choose an edge, allocate egress IPs, or program nftables/SNI routes.

## User lifecycle

An authenticated user with an active subscription and remaining quota can bind the subscription to the public IPv4 address seen by the backend:

```text
POST /api/v1/me/network/entitlement
GET /api/v1/me/network/entitlement
DELETE /api/v1/me/network/entitlement
GET /api/v1/me/network/saved-ips
DELETE /api/v1/me/network/saved-ips?clientIp=<IPv4>
```

`POST` uses the trusted client address derived from the request. An optional JSON body `{"clientIp":"<IPv4>"}` explicitly selects that address. A malformed address returns `400 INVALID_CLIENT_IP`; an address that differs from the trusted request address returns `400 CLIENT_IP_MISMATCH`. Repeating it with the same active subscription and IP is idempotent. Changing IP revokes the previous entitlement and creates a new active record.

Until the network protocol gains a stronger device identity, only one active account may claim a public IP at a time so DNS and traffic usage can be attributed unambiguously.

Successful activations save the address for the authenticated account. The saved-IP endpoint returns `{"ips":["<IPv4>"]}` in most-recently-used order. Deleting a saved IP returns 204 and removes that address from the selector. The active entitlement and operational records are independent of this list.

## Connecting from a different network

An authenticated client can send `POST /api/v1/me/network/ip-approvals` with
`{"clientIp":"<IPv4>"}` to create a ten-minute approval. The response contains
`id`, `clientIp` and `expiresAt`. Each account can have one pending approval.
Creating a new approval replaces the previous one.

On the requested network, the same authenticated account sends
`POST /api/v1/me/network/ip-approvals/confirm` with `{"id":"<approval UUID>"}`.
The backend compares the trusted request IP with the approval's IP.
It consumes the approval and saves the IP in the same transaction as activation.
An expired, replaced, or used approval returns `409 IP_APPROVAL_UNAVAILABLE`.
A request from another public IPv4 returns `400 CLIENT_IP_MISMATCH`.
The saved-IP response includes `detectedIp` when the current request has a public IPv4.

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
