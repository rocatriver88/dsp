#!/usr/bin/env bash
# scripts/qa/10-invite-admin.sh
# Admin creates an invite code with max_uses=1. The code is saved to
# state as `invite_code` for the next step to consume during register.
set -euo pipefail
source "$(dirname "$0")/e2e-env.sh"
source "$(dirname "$0")/lib.sh"

step_start "10-invite-admin"

body='{"max_uses": 1}'
resp=$(curl_admin POST /api/v1/admin/invite-codes "$body")

# Handler documents 201 Created; accept 200 defensively in case of future drift.
if created=$(assert_status 201 "$resp" 2>/dev/null); then
  :
elif created=$(assert_status 200 "$resp" 2>/dev/null); then
  :
else
  fail "create invite code: unexpected response: $(printf '%s' "$resp")"
fi

code=$(json_field "$created" code)
[ -z "$code" ] && fail "invite response missing 'code' field: $created"

save_state invite_code "$code"
step_ok "invite code = $code"
