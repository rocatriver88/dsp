#!/usr/bin/env bash
# scripts/qa/30-topup.sh
# Top up the advertiser balance and verify the new balance reflects
# the top-up amount. Runs after 20-register.sh has stored
# advertiser_id + api_key in $QA_STATE_DIR.
#
# Handler references (internal/handler/billing.go):
#   POST /api/v1/billing/topup        body: {advertiser_id, amount_cents, description}
#                                     resp: 200 billing.Transaction JSON
#   GET  /api/v1/billing/balance/{id} resp: 200 {advertiser_id, balance_cents, billing_type}
set -euo pipefail
source "$(dirname "$0")/e2e-env.sh"
source "$(dirname "$0")/lib.sh"

step_start "30-topup"

advID=$(load_state advertiser_id)
api_key=$(load_state api_key)
[ -z "$advID" ] && fail "advertiser_id state empty"
[ -z "$api_key" ] && fail "api_key state empty"

amount=500000

# Read the starting balance so we can assert the delta.
before_resp=$(curl_json GET "/api/v1/billing/balance/${advID}" "" "$api_key")
before_body=$(assert_status 200 "$before_resp") || fail "GET balance (before)"
before=$(json_field "$before_body" balance_cents)
[ -z "$before" ] && before=0
step_ok "balance before topup: ${before}c"

# Perform the top-up. Handler docs confirm 200 on success.
body=$(cat <<EOF
{
  "advertiser_id": ${advID},
  "amount_cents": ${amount},
  "description": "qa harness topup"
}
EOF
)
topup_resp=$(curl_json POST /api/v1/billing/topup "$body" "$api_key")
topup_body=$(assert_status 200 "$topup_resp") || fail "POST /billing/topup"

# Spot-check a couple of top-level fields on the transaction payload
# to catch silent shape drift. Missing fields just log, they don't fail.
txn_id=$(json_field "$topup_body" id)
txn_amount=$(json_field "$topup_body" amount_cents)
if [ -n "$txn_id" ] && [ -n "$txn_amount" ]; then
  step_ok "topup txn id=${txn_id} amount=${txn_amount}c"
else
  log "warning: topup response missing id/amount_cents: $topup_body"
fi

# Confirm the balance moved by at least the amount we topped up.
# Other test runs or background jobs may add more, so use >= not ==.
after_resp=$(curl_json GET "/api/v1/billing/balance/${advID}" "" "$api_key")
after_body=$(assert_status 200 "$after_resp") || fail "GET balance (after)"
after=$(json_field "$after_body" balance_cents)
[ -z "$after" ] && fail "balance after topup missing: $after_body"

expected_min=$((before + amount))
if [ "$after" -lt "$expected_min" ]; then
  fail "balance did not rise: before=${before} after=${after} expected>=${expected_min}"
fi
step_ok "balance after topup: ${after}c (>= ${expected_min})"

save_state topup_amount "$amount"
save_state balance_after_topup "$after"
step_ok "30-topup done"
