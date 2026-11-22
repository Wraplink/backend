# Public IPv4 network synchronization contract (draft)

This contract is a prerequisite for enabling the public-IP reconciliation worker.
The current backend PR stores desired state, but **does not activate an IP**.

## Revision identity

Each request is identified by `(subscription_id, revision, public_ip_id)`.
An attempt additionally has a unique `attempt_id`. A revision is monotonically
increasing per subscription. The control plane must reject a revision lower
than the highest durable revision already accepted for that subscription,
including after process restart. A repeated identical revision must be
idempotent. A repeated revision with a different IP is a conflict.

## Apply and acknowledge

1. Verify the subscription still belongs to the user and the selected address
   is verified and not removed.
2. Persist desired revision and attempt identity at the control plane before
   distributing work. Do not revoke the previously working address yet.
3. DNS Control must report the **applied** revision for the relevant policy
   generation, not merely the revision it received.
4. Each required Edge Agent must report its **applied** revision and effective
   enforcement state. Heartbeat, registration, and configuration download are
   insufficient.
5. The backend may acknowledge the revision only after all required nodes
   report the exact subscription, IP, revision, and successful enforcement.
   An absent or unreachable node means unconfirmed, not success.
6. Retire the previous address only after the new address is confirmed and
   the control plane can safely remove the previous entitlement.

## Recovery and fencing

An expired attempt or ambiguous network error enters `needs_reconciliation`.
A normal retry must not release it. The verifier must inspect every required
node and durably fence the old attempt before permitting a retry. It must
reject missing acknowledgements, conflicting IPs, mismatched revisions,
unreachable nodes, and stale attempts.

A node must reject operations from superseded attempts even when an older
operation arrives after a newer one. Database-only fencing does **not**
prevent stale network side effects.

## Required tests before production

- Duplicate apply for the same revision is idempotent.
- Same revision with different IP is rejected.
- Older revision delivered after newer revision is rejected, including
  after an agent restart.
- Partial application and network timeout remain quarantined.
- Missing DNS or Edge acknowledgement prevents backend acknowledgement.
- Previous working IP remains usable until replacement is confirmed.
- Concurrent selection, removal, expiry, and retry cannot bypass fencing.
- Restart and replay preserve durable revision and acknowledgement state.
- End-to-end tests cover actual DNS answers and edge enforcement.

## Rollout gate

Do not schedule `ReconcileOne`, wire a permissive verifier, or expose
selection as activation until DNS Control and Edge Agent implement and
persist the acknowledgement and fencing contract. CI passing for backend
unit tests is not sufficient evidence of production readiness.
