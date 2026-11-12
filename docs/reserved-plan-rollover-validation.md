# Reserved-plan rollover: production validation

The Backend implementation in PR #99 rebinds an existing connected IP when
an exhausted or expired subscription has a usable reserved successor.

## Automated checks

The PostgreSQL integration test `TestReservedEntitlementRolloverPostgres`
requires a **disposable, fully migrated** database and `TEST_DATABASE_URL`.
It covers promotion, repeated and concurrent snapshots/status reads, delayed
usage reports, concurrent post-rollover reports, cross-boundary rejection,
and the absence of a usable successor.

Run `go test ./internal/networkentitlement -run TestReservedEntitlementRolloverPostgres -count=1 -v`
against the disposable database. Never use production credentials.

## Known accounting limitation — release blocker

An Edge usage report contains aggregate uploaded/downloaded bytes and a
[startedAt, endedAt] interval. When that interval spans an entitlement switch,
Backend cannot determine the bytes belonging to each subscription. It currently
rejects the report rather than assigning its bytes to the wrong plan. This
protects attribution but **does not preserve accounting completeness**.

Do not distribute report bytes by elapsed-time proportion: throughput may vary
widely inside the interval. A correct fix requires measured byte deltas split
at an entitlement boundary (or a protocol that reports trustworthy cumulative
counters at that boundary), with durable idempotent delivery.

## Edge Agent requirements

- SNI: verify session start/end timestamps and report intervals when a
  session remains open across entitlement replacement.
- Full Route: its accounting status document explicitly says runtime counter
  integration and crash-safe checkpoint/outbox persistence are incomplete.
  Do not claim Full Route billing support until those are implemented.
- Ensure delayed reports for an interval entirely before rollover still
  attribute to the historical entitlement.
- Confirm a replacement entitlement reaches DNS-Control and Edge Agent,
  retaining the connected client IP, without a manual Connect action.

## Production acceptance

1. On a disposable database, exercise the complete sequence from an active
   subscription with finite remaining bytes through an Edge usage report
   that exhausts it and automatic promotion of a reserved plan.
2. Verify a connected client remains authorized across the next DNS/Edge
   synchronization cycle and the active subscription ID changes exactly once.
3. Repeat with no reserved plan: authorization must cease and Connect should
   show the localized purchase-plan message.
4. Repeat with an unlimited plan: usage never triggers quota exhaustion.
5. Exercise concurrent duplicate reports, reporting during rollover,
   delayed historical reports, and cross-boundary intervals.
6. Validate the real production container images and configuration on a
   disposable environment before touching production.

Do not merge/deploy as production-ready while cross-boundary accounting or
Full Route billing requirements remain unresolved.
