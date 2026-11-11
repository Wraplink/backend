#!/usr/bin/env bash
# Read-only production preflight. Does not activate, claim, register or mutate.
set -euo pipefail
: "${WRAPLINK_API_URL:?Set HTTPS API origin}"
: "${WRAPLINK_TEST_TOKEN:?Set a test user's bearer token}"
base="${WRAPLINK_API_URL%/}"
[[ "$base" =~ ^https://[a-zA-Z0-9.-]+(:[0-9]{1,5})?$ ]] || { echo "Expected bare HTTPS API origin" >&2; exit 2; }
command -v curl >/dev/null; command -v jq >/dev/null
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
auth=(-H "Authorization: Bearer $WRAPLINK_TEST_TOKEN" -H "Accept: application/json")
check() {
 local path="$1" out="$2" status
 status="$(curl --silent --show-error --max-time 20 --request GET "${auth[@]}" -o "$out" -w '%{http_code}' "$base$path")"
 [[ "$status" == 200 ]] || { echo "GET $path returned HTTP $status" >&2; return 1; }
 jq -e . "$out" >/dev/null || { echo "GET $path returned invalid JSON" >&2; return 1; }
 echo "PASS: GET $path (HTTP 200)"
}
check /api/v1/me/free-plans/claims "$tmp/free.json"
jq -e '(.claims|type)=="array" and all(.claims[]; (.planCode|type)=="string" and (.source|type)=="string" and (.state|type)=="string")' "$tmp/free.json" >/dev/null
check /api/v1/me/starter-claims "$tmp/starter.json"
jq -e '(.claims|type)=="array" and all(.claims[]; (.id|type)=="string" and (.state|type)=="string")' "$tmp/starter.json" >/dev/null
echo "PASS: read-only free-plan and starter-claim API contract checks."
echo "No POST, PATCH, PUT, DELETE or subscription activation performed."
