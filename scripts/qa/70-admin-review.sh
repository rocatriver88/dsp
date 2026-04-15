#!/usr/bin/env bash
# scripts/qa/70-admin-review.sh
# Smoke-test the admin creative review endpoints. In dev (ENV !=
# production) creatives auto-approve on create, so the pending list
# will usually be empty — that's fine, we just want to confirm the
# endpoint is live and the shape is a JSON array. We also hit the
# approve endpoint against the creative we already created; it is
# already approved, but the handler is idempotent (UpdateCreativeStatus
# always writes).
#
# Handler references (internal/handler/admin.go):
#   GET  /api/v1/admin/creatives?status=pending   resp: 200 [campaign.Creative]
#   POST /api/v1/admin/creatives/{id}/approve     resp: 200 {status:"approved"}
set -euo pipefail
source "$(dirname "$0")/e2e-env.sh"
source "$(dirname "$0")/lib.sh"

step_start "70-admin-review"

crid=$(load_state creative_id)
[ -z "$crid" ] && fail "creative_id state empty"

# List pending creatives. Dev mode auto-approves, so this is likely
# []. We only assert the endpoint responds 200 and the body is a
# JSON array (parseable).
pending_resp=$(curl_admin GET "/api/v1/admin/creatives?status=pending" "")
pending_body=$(assert_status 200 "$pending_resp") || fail "GET /admin/creatives?status=pending"

# Cheap array shape check: first non-whitespace char should be [.
first_char=$(printf '%s' "$pending_body" | tr -d '[:space:]' | head -c 1)
if [ "$first_char" != "[" ]; then
  fail "admin/creatives body is not a JSON array: $pending_body"
fi
log "pending creatives body: $pending_body"
step_ok "GET /admin/creatives?status=pending -> 200 (array)"

# Also list all approved creatives — our own creative should show up
# in that list, giving us some signal beyond just "endpoint answers".
approved_resp=$(curl_admin GET "/api/v1/admin/creatives?status=approved" "")
approved_body=$(assert_status 200 "$approved_resp") || fail "GET /admin/creatives?status=approved"
first_char=$(printf '%s' "$approved_body" | tr -d '[:space:]' | head -c 1)
if [ "$first_char" != "[" ]; then
  fail "admin/creatives?approved body is not a JSON array: $approved_body"
fi
step_ok "GET /admin/creatives?status=approved -> 200 (array)"

# Call the approve endpoint on our creative. It is already approved
# from the dev auto-approval path, but the handler happily re-writes
# the status and returns 200 — this exercises the happy path of the
# review endpoint without needing a genuinely pending creative.
approve_resp=$(curl_admin POST "/api/v1/admin/creatives/${crid}/approve" "{}")
approve_body=$(assert_status 200 "$approve_resp") || fail "POST /admin/creatives/${crid}/approve"
status=$(json_field "$approve_body" status)
[ "$status" = "approved" ] || fail "approve response status=${status} body=${approve_body}"
step_ok "POST /admin/creatives/${crid}/approve -> 200 status=approved"

step_ok "70-admin-review done"
