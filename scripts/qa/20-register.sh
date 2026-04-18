#!/usr/bin/env bash
# scripts/qa/20-register.sh
# Submit a public registration using the invite code from step 10,
# then admin-approve it. Capture advertiser_id + api_key into state
# so downstream steps (30-topup.sh, 40-campaign.sh, ...) can act as
# the new advertiser.
#
# Handler references (internal/handler/admin.go):
#   POST /api/v1/register                          -> HandleRegister (201)
#   GET  /api/v1/admin/registrations               -> HandleListRegistrations
#   POST /api/v1/admin/registrations/{id}/approve  -> HandleApproveRegistration
#     response shape: {advertiser_id, api_key, user_email, temp_password, message}
set -euo pipefail
source "$(dirname "$0")/e2e-env.sh"
source "$(dirname "$0")/lib.sh"

step_start "20-register"

invite=$(load_state invite_code)
[ -z "$invite" ] && fail "invite_code state empty; 10-invite-admin.sh must run first"

ts=$(date +%s)
# Randomize the email so repeated runs (without teardown) don't hit
# the duplicate-email 409 path. $$ adds the shell pid for extra safety
# against same-second reruns.
email="qa-harness-${ts}-$$@test.local"
company="QA Harness ${ts}"

body=$(cat <<EOF
{
  "company_name": "${company}",
  "contact_email": "${email}",
  "invite_code": "${invite}"
}
EOF
)

resp=$(curl_json POST /api/v1/register "$body")
# Handler documents 201 Created; accept 200 defensively.
if registered=$(assert_status 201 "$resp" 2>/dev/null); then
  :
elif registered=$(assert_status 200 "$resp" 2>/dev/null); then
  :
else
  fail "register submit: unexpected response: $(printf '%s' "$resp")"
fi
step_ok "registration submitted: $email"

# Look up the pending registration id via admin list.
list_resp=$(curl_admin GET /api/v1/admin/registrations "")
list_body=$(assert_status 200 "$list_resp") || fail "list pending registrations"

reg_id=$(json_array_find_field "$list_body" contact_email "$email" id)
[ -z "$reg_id" ] && fail "new registration not in admin pending list (email=$email) body=$list_body"

# Approve — handler tolerates empty body, but send "{}" to be safe for
# any Content-Length sniffing proxies.
approve_resp=$(curl_admin POST "/api/v1/admin/registrations/${reg_id}/approve" "{}")
approve_body=$(assert_status 200 "$approve_resp") || fail "approve registration $reg_id"

# Read advertiser_id + api_key directly from the approve response.
# internal/handler/admin.go:HandleApproveRegistration returns
#   {"advertiser_id": <int>, "api_key": "<string>",
#    "user_email": "<string>", "temp_password": "<string>", "message": "..."}
# The QA chain only needs advertiser_id + api_key (the user seeding /
# temp_password path is covered by test/e2e/test_e2e_flow.py Step 3/4).
adv_id=$(json_field "$approve_body" advertiser_id)
api_key=$(json_field "$approve_body" api_key)

# Fallback: if approve didn't include the api_key (unexpected — would
# indicate handler drift), query Postgres directly. The admin list-
# advertisers endpoint was redacted in P2.8b and no longer returns
# api_key, so psql is the only reliable fallback path.
if [ -z "$api_key" ]; then
  log "approve did not return api_key -- falling back to postgres direct query"
  api_key=$(docker compose -p dsp-biz exec -T postgres psql -U dsp -d dsp -tA \
    -c "SELECT api_key FROM advertisers WHERE contact_email = '${email}'" \
    | tr -d '[:space:]')
fi
[ -z "$api_key" ] && fail "could not obtain api_key for $email"

if [ -z "$adv_id" ]; then
  log "approve did not return advertiser_id -- falling back to postgres direct query"
  adv_id=$(docker compose -p dsp-biz exec -T postgres psql -U dsp -d dsp -tA \
    -c "SELECT id FROM advertisers WHERE contact_email = '${email}'" \
    | tr -d '[:space:]')
fi
[ -z "$adv_id" ] && fail "could not obtain advertiser_id for $email"

save_state advertiser_id "$adv_id"
save_state api_key "$api_key"
save_state contact_email "$email"
step_ok "approved: advertiser_id=$adv_id email=$email api_key=${api_key:0:12}..."
