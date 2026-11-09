# Starter plan contract (design and implementation checklist)

Tracks #90 and #88. This is a contract/design document, not an implemented endpoint or migration.

## Semantics
- `price_minor=0` means free; never require payment. A starter plan is a free plan with `kind=starter`.
- **One claim per account per plan for life**, including expired/cancelled subscriptions. Retain claim tombstones if plans are archived/deleted.
- Assignment is not activation. Default policy: after verified email, reserve one starter offer; user explicitly activates it. Duration (e.g. 60 minutes) begins at activation, not verification.
- Reservation expires after configurable TTL (default 7 days). Expired reservations remain claimed: do not reissue automatically. The administrator may configure a different policy.
- Eligibility for default starter offer: newly verified user only; do not retroactively assign to existing users without explicit admin backfill.
- At most one automatically assigned starter offer per onboarding cohort. Sort enabled policies by priority descending, then stable policy ID, and choose first matching region/audience. Prevent policy overlaps at save time where feasible.
- No payment, wallet debit, receipt or paid checkout path. Reuse subscription entitlement and agent synchronization.
- Rate-limit account registration, require verified email, audit decisions; CAPTCHA is complementary, not a substitute for server-side eligibility.

## Proposed schema (adapt names/types to existing migrations)
```sql
CREATE TABLE free_plan_claims (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL,
  plan_id uuid NOT NULL,
  source text NOT NULL CHECK (source IN ('market','starter','admin')),
  state text NOT NULL CHECK (state IN ('reserved','activating','active','expired','cancelled','failed')),
  assigned_at timestamptz NOT NULL DEFAULT now(),
  reservation_expires_at timestamptz,
  activated_at timestamptz,
  subscription_id uuid,
  UNIQUE (user_id, plan_id)
);
-- Preserve historical claims even if plans are archived; do not cascade-delete them.
```
Use actual FK types and deletion semantics after inspecting existing schema. For historical free subscriptions, backfill claim ledger before enforcing uniqueness.

## Suggested API contract (to reconcile with existing route conventions)
- `GET /api/v1/me/starter-plan` -> `{plan, status: "none|reserved|activating|active|expired|claimed", assignedAt, reservationExpiresAt, activatedAt, expiresAt}`
- `POST /api/v1/me/starter-plan/activate` with `Idempotency-Key` -> existing subscription summary; 409 `starter_plan_already_claimed` or `starter_plan_reservation_expired`.
- `GET/POST/PATCH /api/v1/admin/starter-plans` -> admin plan management; force price zero.
- `GET/POST/PATCH /api/v1/admin/starter-policies` -> `enabled, priority, region, audience, assignTrigger, activateTrigger, reservationTTL, planId`.
- Existing market claim endpoint must consult the same ledger; never bypass via basket or checkout.

## Transaction boundaries
1. On email verification: idempotently choose matching policy and insert reserved claim with UNIQUE(user_id,plan_id); ensure only one auto-offer per onboarding event. Never call remote agent inside database transaction.
2. On activation: lock claim row, verify reservation/eligibility, create subscription and mark activating in same transaction; enqueue provisioning via outbox (or existing durable job).
3. On provisioning success: mark active and record activated_at; expiry timestamp must use intended activation start and policy duration, not time of retry.
4. On failure: retain claim and retry provisioning idempotently. Do not create another subscription or consume a second claim.
5. On expiry: revoke entitlements; preserve claim history. Archiving a plan does not reset claims.

## Acceptance tests
- Registration without verification: no claim.
- Verification once/replayed/concurrently: one reservation.
- New account after verification: reserved plan visible without payment.
- Activation twice/concurrently: one subscription, stable idempotent result or documented conflict.
- Trial lasts exactly configured duration from activation, not assignment.
- Reservation expiry, disabled policy, region matching, priority tie, plan archive, cancelled/expired subscription, basket bypass.
- Agent provisioning timeout/retry, delayed ack, entitlement revoke, historical claim backfill.
- Existing paid plans and ordinary free plans unaffected.
