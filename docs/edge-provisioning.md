# Edge provisioning protocol

## Purpose
The backend owns desired access state. Edge agents poll revisioned events and apply them to nftables / routing locally. Edge agents do not read PostgreSQL directly.

## Configuration

```shell
export NETWORK_EDGE_SYNC_KEY="$(openssl rand -hex 32)"
```

`network.edge-sync-key` enables internal edge routes. `network.idle-release-after`, `network.edge-offline-after`, and `network.reconcile-interval` control automatic cleanup.

## Register an edge

```http
POST /api/v1/internal/edges/register
X-Edge-Key: <NETWORK_EDGE_SYNC_KEY>
Content-Type: application/json

{
  "code": "edge-eu-1",
  "region": "eu",
  "publicIps": ["203.0.113.10"],
  "metadata": {}
}
```

Registration is idempotent. Public IPs already owned by another edge are rejected.

## User provisioning
An authenticated user with an active subscription and remaining quota requests allocation:

```http
POST /api/v1/me/network/provision?region=eu
Authorization: Bearer <access-token>
```

The backend captures the trusted client IP, reserves one available edge egress IP, creates a `provisioning` assignment, and emits an `access.upsert` event.

## Edge sync loop

1. Send a heartbeat periodically:

```http
POST /api/v1/internal/edges/edge-eu-1/heartbeat
X-Edge-Key: <NETWORK_EDGE_SYNC_KEY>
```

2. Poll after the last successfully processed revision:

```http
GET /api/v1/internal/edges/edge-eu-1/sync?after=0&limit=200
X-Edge-Key: <NETWORK_EDGE_SYNC_KEY>
```

3. Apply each event in revision order. `access.upsert` contains `clientIp` and `egressIp`; `access.remove` removes the corresponding access/routing state.

4. Acknowledge only after the local change succeeds:

```http
POST /api/v1/internal/edges/edge-eu-1/ack
X-Edge-Key: <NETWORK_EDGE_SYNC_KEY>
Content-Type: application/json

{
  "revision": 1,
  "success": true
}
```

A successful upsert acknowledgement changes the assignment from `provisioning` to `active`. A successful remove acknowledgement changes it to `released` and returns the egress IP to the available pool.

## Activity and idle release
Edge agents periodically report recently active assignments:

```http
POST /api/v1/internal/edges/edge-eu-1/activity
X-Edge-Key: <NETWORK_EDGE_SYNC_KEY>
Content-Type: application/json

{
  "items": [
    {"accessId": "<assignment-uuid>"}
  ]
}
```

The backend reconciler emits `access.remove` when an assignment exceeds the idle timeout, its subscription expires, or its quota is exhausted. Nodes missing heartbeats are marked offline and excluded from new allocations.

## Client IP changes
Calling the provision endpoint again for the same active assignment updates the client IP and emits another `access.upsert`. This lets the edge replace the old source-IP rule without allocating a second egress IP.
