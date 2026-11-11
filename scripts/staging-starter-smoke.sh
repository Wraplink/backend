#!/usr/bin/env bash
# Staging-only smoke test for starter claims. Requires curl, jq and a disposable
# verified user's bearer token. Does not register users or alter production data.
set -euo pipefail
: "${WRAPLINK_API_URL:?Set staging API base URL}"
: "${WRAPLINK_TEST_TOKEN:?Set disposable verified user bearer token}"
case "$WRAPLINK_API_URL" in
  https://*) ;;
  *) echo "Refusing non-HTTPS API URL" >&2; exit 2 ;;
esac
base="${WRAPLINK_API_URL%/}"
auth=(-H "Authorization: Bearer $WRAPLINK_TEST_TOKEN" -H "Accept: application/json")
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
request() {
 local method="$1" path="$2" output="$3"
 curl --fail-with-body --silent --show-error --max-time 20 \
  -X "$method" "${auth[@]}" -o "$output" "$base$path"
}
request GET /api/v1/me/starter-claims "$tmp/before.json"
jq -e '(.claims|type=="array") and (.claims|length)==1 and .claims[0].state=="reserved" and (.claims[0].subscriptionId==null)' "$tmp/before.json" >/dev/null
claim_id="$(jq -r '.claims[0].id' "$tmp/before.json")"
[[ "$claim_id" =~ ^[0-9a-fA-F-]{36}$ ]] || { echo "Invalid claim ID" >&2; exit 1; }
echo "Reservation verified; claim ID: $claim_id"
if [[ "${WRAPLINK_ACTIVATE_TRIAL:-}" != "YES" ]]; then
 echo "Read-only checks passed. Set WRAPLINK_ACTIVATE_TRIAL=YES to activate disposable trial."
 exit 0
fi
request POST "/api/v1/me/starter-claims/$claim_id/activate" "$tmp/activated.json"
jq -e --arg claim "$claim_id" '.claimId==$claim and (.subscriptionId|type=="string") and (.startsAt|type=="string") and (.expiresAt|type=="string")' "$tmp/activated.json" >/dev/null
request POST "/api/v1/me/starter-claims/$claim_id/activate" "$tmp/retry.json"
jq -e --arg sub "$(jq -r '.subscriptionId' "$tmp/activated.json")" '.subscriptionId==$sub' "$tmp/retry.json" >/dev/null
request GET /api/v1/me/starter-claims "$tmp/after.json"
jq -e --arg claim "$claim_id" --arg sub "$(jq -r '.subscriptionId' "$tmp/activated.json")" 'any(.claims[]; .id==$claim and .subscriptionId==$sub and .state=="active")' "$tmp/after.json" >/dev/null
echo "Activation and idempotent retry passed."
if [[ "${WRAPLINK_CHECK_EXPIRY:-}" == "YES" ]]; then
 : "${WRAPLINK_EXPIRY_TIMEOUT_SECONDS:=180}"
 [[ "$WRAPLINK_EXPIRY_TIMEOUT_SECONDS" =~ ^[0-9]+$ ]] && (( WRAPLINK_EXPIRY_TIMEOUT_SECONDS > 0 && WRAPLINK_EXPIRY_TIMEOUT_SECONDS <= 3600 )) || {
  echo "Expiry timeout must be an integer between 1 and 3600 seconds" >&2; exit 2;
 }
 deadline=$((SECONDS + WRAPLINK_EXPIRY_TIMEOUT_SECONDS))
 expired=false
 while (( SECONDS < deadline )); do
  request GET /api/v1/me/starter-claims "$tmp/expired.json"
  if jq -e --arg claim "$claim_id" 'any(.claims[]; .id==$claim and .state=="expired")' "$tmp/expired.json" >/dev/null; then
   expired=true
   break
  fi
  sleep 2
 done
 [[ "$expired" == "true" ]] || { echo "Trial did not become expired before timeout" >&2; exit 1; }
 status="$(curl --silent --show-error --max-time 20 -o "$tmp/rejected.json" -w '%{http_code}' -X POST "${auth[@]}" "$base/api/v1/me/starter-claims/$claim_id/activate")"
 [[ "$status" == "409" ]] || { echo "Expired activation returned HTTP $status, expected 409" >&2; exit 1; }
 echo "Expired trial is displayed as expired and cannot be reactivated."
fi
echo "DNS/Edge entitlement propagation and expiry revocation still require separate verification."
