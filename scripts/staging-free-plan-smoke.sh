#!/usr/bin/env bash
# Destructive staging-only E2E: claim a zero-price market plan once.
# Requires a disposable verified account without an active subscription.
set -euo pipefail
: "${WRAPLINK_API_URL:?Set staging HTTPS API origin}"
: "${WRAPLINK_STAGING_HOST:?Set exact staging hostname}"
: "${WRAPLINK_STAGING_CONFIRMED:?Set YES to allow one-time claim}"
: "${WRAPLINK_TEST_TOKEN:?Set disposable user's bearer token}"
: "${WRAPLINK_FREE_PLAN_CODE:?Set published zero-price plan code}"
[[ "$WRAPLINK_STAGING_CONFIRMED" == YES ]] || { echo "Staging confirmation must be YES" >&2; exit 2; }
base="${WRAPLINK_API_URL%/}"
[[ "$base" =~ ^https://[a-zA-Z0-9.-]+(:[0-9]{1,5})?$ ]] || { echo "Only a bare HTTPS origin is allowed" >&2; exit 2; }
host="${base#https://}"; host="${host%%:*}"
[[ "$host" == "$WRAPLINK_STAGING_HOST" ]] || { echo "Staging hostname mismatch" >&2; exit 2; }
case "$host" in api.wraplink.ir|wraplink.ir|www.wraplink.ir) echo "Production hostname refused" >&2; exit 2;; esac
command -v curl >/dev/null; command -v jq >/dev/null
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
auth=(-H "Authorization: Bearer $WRAPLINK_TEST_TOKEN" -H "Accept: application/json")
call() {
 local method="$1" path="$2" file="$3"; shift 3
 curl --silent --show-error --max-time 20 -o "$file" -w '%{http_code}' \
  -X "$method" "${auth[@]}" "$@" "$base$path"
}
code="$(printf '%s' "$WRAPLINK_FREE_PLAN_CODE" | tr '[:upper:]' '[:lower:]')"
before="$(call GET /api/v1/me/free-plans/claims "$tmp/before")"
[[ "$before" == 200 ]] || { echo "Claims listing HTTP $before" >&2; exit 1; }
jq -e --arg code "$code" '(.claims|type=="array") and (all(.claims[]; (.planCode|ascii_downcase)!=$code))' "$tmp/before" >/dev/null || {
 echo "Account already claimed this plan; use a fresh disposable account" >&2; exit 1;
}
# Refuse to consume a one-time claim when the account already has a live subscription.
# This also makes a failed claim easier to diagnose before any mutation.
pre_current="$(call GET /api/v1/me/subscriptions/current "$tmp/pre-current")"
[[ "$pre_current" == 404 ]] || {
 echo "Expected no current subscription before claiming (HTTP $pre_current)" >&2
 exit 1
}
payload="$(jq -nc --arg code "$WRAPLINK_FREE_PLAN_CODE" '{planCode:$code}')"
first="$(call POST /api/v1/me/free-plans/claim "$tmp/first" -H 'Content-Type: application/json' --data "$payload")"
[[ "$first" == 200 || "$first" == 201 ]] || { echo "Initial claim HTTP $first" >&2; exit 1; }
jq -e '(.claimId|type=="string") and (.subscriptionId|type=="string") and (.startsAt|type=="string") and (.expiresAt|type=="string")' "$tmp/first" >/dev/null
sub="$(jq -r '.subscriptionId' "$tmp/first")"
current="$(call GET /api/v1/me/subscriptions/current "$tmp/current")"
[[ "$current" == 200 ]] || { echo "Current subscription HTTP $current" >&2; exit 1; }
jq -e --arg id "$sub" '.id==$id and .status=="active" and .priceMinor==0' "$tmp/current" >/dev/null
second="$(call POST /api/v1/me/free-plans/claim "$tmp/second" -H 'Content-Type: application/json' --data "$payload")"
[[ "$second" == 409 ]] || { echo "Duplicate claim HTTP $second; expected 409" >&2; exit 1; }
after="$(call GET /api/v1/me/free-plans/claims "$tmp/after")"
[[ "$after" == 200 ]] || { echo "Claims recheck HTTP $after" >&2; exit 1; }
jq -e --arg code "$code" '
 [.claims[]|select((.planCode|ascii_downcase)==$code)] as $matches |
 ($matches|length)==1 and $matches[0].source=="market" and $matches[0].state=="active"
' "$tmp/after" >/dev/null
echo "PASS: free claim, active zero-price subscription, duplicate rejection, durable claim."
if [[ "${WRAPLINK_CHECK_FREE_EXPIRY:-}" == YES ]]; then
 : "${WRAPLINK_EXPIRY_TIMEOUT_SECONDS:=180}"
 [[ "$WRAPLINK_EXPIRY_TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] && (( WRAPLINK_EXPIRY_TIMEOUT_SECONDS >= 1 && WRAPLINK_EXPIRY_TIMEOUT_SECONDS <= 3600 )) || {
  echo "Expiry timeout must be 1..3600 seconds" >&2; exit 2;
 }
 deadline=$((SECONDS + WRAPLINK_EXPIRY_TIMEOUT_SECONDS))
 expired=false
 while (( SECONDS < deadline )); do
  status="$(call GET /api/v1/me/subscriptions/current "$tmp/expired")"
  if [[ "$status" == 404 ]]; then expired=true; break; fi
  [[ "$status" == 200 ]] || { echo "Unexpected current subscription HTTP $status" >&2; exit 1; }
  sleep 2
 done
 [[ "$expired" == true ]] || { echo "Subscription did not expire before timeout" >&2; exit 1; }
 again="$(call POST /api/v1/me/free-plans/claim "$tmp/after-expiry" -H 'Content-Type: application/json' --data "$payload")"
 [[ "$again" == 409 ]] || { echo "Expired plan re-claim HTTP $again; expected 409" >&2; exit 1; }
 echo "PASS: expired subscription removed from current view; lifetime re-claim blocked."
fi
echo "NOT VERIFIED: concurrent claims, DNS/Edge entitlement propagation and revocation."
