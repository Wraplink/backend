# Starter and one-time free plans — release verification

This is a **manual staging acceptance test**, not evidence that the scenarios have passed.
Run with a disposable verified account, a test agent and DNS controller, and a short
starter policy (for example 60 seconds). Do not use production customer accounts.

## Preconditions
- Apply migrations 000090 and 000091, and deploy backend, web, dns-control and edge-agent.
- Ensure the agent heartbeat is healthy and a test client IP is authorized.
- Create a published, active, zero-price plan and enable a global starter policy with
  `assignment_trigger=email_verified`, `activation_trigger=manual`, a short trial
  duration and reservation TTL.
- Record the test account, plan ID/code, policy ID, agent ID and timestamps.

## Onboarding and reservation
1. Register a **new** account and confirm no active subscription or network entitlement
   is created before email verification.
2. Verify the email once. `GET /api/v1/me/starter-claims` must return exactly one
   `reserved` starter claim with a future `reservationExpiresAt`.
3. Reuse the verification link and retry the verification request: no second claim.
4. Confirm the dashboard displays the activation button and reservation countdown.
5. Before activation, verify the test client receives **no new** starter entitlement
   at the edge and cannot use a paid/privileged route on the strength of the reservation.

## Activation and enforcement
6. POST `/api/v1/me/starter-claims/{claimID}/activate` as the claim owner.
   Expect a subscription ID, `startsAt` near activation time, and `expiresAt`
   equal to the configured trial duration later (allow clock skew).
7. Retry immediately: expect the same subscription, never a second subscription.
8. Retry with a different user: reject; never disclose or activate another user's claim.
9. Verify subscription/entitlement appears in backend and propagates through
   dns-control and edge-agent. Test an authorized DNS query and real routed traffic.
   Confirm traffic counters increase on the **correct** user and route.
10. While another active subscription exists, activating a reserved starter must
    fail with conflict and leave its claim reserved (subject to reservation expiry).

## Expiration, cancellation and abuse
11. After trial expiration, verify the backend no longer treats it as an active
    subscription; DNS and edge must stop granting the trial entitlement.
    Test both DNS answer and an **already established** traffic flow.
12. Confirm dashboard shows expired, and a second activation cannot extend the trial.
13. Cancel a different active trial: the dashboard must show cancelled; retry must
    not reactivate it.
14. Wait past an unused reservation TTL: activation must fail and no entitlement
    may be issued. Verify a replacement starter cannot be automatically awarded.
15. In Market, claim an eligible zero-price plan directly without payment.
    Confirm it creates one active subscription and one lifetime claim.
16. Retry the same claim concurrently from two sessions: at most one succeeds.
    Repeat after expiry/cancellation, after plan archive/delete/recreate with the
    same code, and using different case in the plan code: all must be rejected.
17. Confirm a user with unverified email gets HTTP 403 and the bilingual verification
    message. Anonymous users must be directed to sign in.
18. Verify the normal basket/checkout cannot activate a zero-price plan without
    passing through the lifetime-claim path.

## Race and rollback checks
19. At the exact reservation expiry boundary, race two activation requests. The final claim update must reject an expired reservation and roll back any inserted subscription. Verify claim and subscription counts remain consistent.
20. Disable or archive the zero-price plan between initial eligibility inspection and subscription creation. The claim transaction must fail without leaving a durable claim or subscription.
21. Corrupt a disposable staging subscription to have null or future start/end timestamps (only if staging schema permits), then retry activation. Expect conflict, never an extension or a new subscription.
22. Confirm a user cannot obtain a second entitlement by mixing a starter reservation with a direct Market free claim while another subscription is active.

## Evidence to capture
- API request IDs, responses and HTTP statuses for steps 2, 6, 7, 12, 15 and 16.
- Database snapshots of `free_plan_claims` and `user_subscriptions` (omit PII).
- DNS controller synchronization revision and agent entitlement revision before
  activation, after activation and after expiry.
- Edge route/DNS test results and traffic counter deltas.
- CI links and deployment image digests for each service.

**Release gate:** Do not mark the starter/free-plan PRs ready solely because unit
tests and migrations pass. Steps 1–22 must be verified against a staging deployment,
especially the network entitlement removal at expiration.
