#!/usr/bin/env bash
# scripts/qa/40-campaign.sh
# Create a draft campaign for the advertiser and verify the update
# path publishes on Redis pub/sub. The campaign starts in status=draft
# and is left that way — 50-creative.sh attaches a creative and
# transitions it to active.
#
# Handler references (internal/handler/campaign.go):
#   POST /api/v1/campaigns           body: {name, billing_model, budget_total_cents,
#                                           budget_daily_cents, bid_cpm_cents, targeting}
#                                    resp: 201 {id, status:"draft"}
#                                    pub:  campaign:updates action="updated"
#   PUT  /api/v1/campaigns/{id}      body: {name, bid_cpm_cents, budget_daily_cents, targeting}
#                                    resp: 200 {status:"updated"}
#                                    pub:  campaign:updates action="updated"
set -euo pipefail
source "$(dirname "$0")/e2e-env.sh"
source "$(dirname "$0")/lib.sh"

step_start "40-campaign"

advID=$(load_state advertiser_id)
api_key=$(load_state api_key)
[ -z "$advID" ] && fail "advertiser_id state empty"
[ -z "$api_key" ] && fail "api_key state empty"

sub_log="${QA_STATE_DIR}/updates-40-campaign.log"
: > "$sub_log"

# Subscribe before any writes so we catch both the create and update
# publishes. redis_cmd blocks, so run it in the background.
redis_cmd SUBSCRIBE campaign:updates > "$sub_log" 2>&1 &
sub_pid=$!
# Wait for the subscribe to actually register with redis. When
# redis_cmd falls back to `docker compose exec`, there's ~1s of
# container exec overhead before the SUBSCRIBE command even runs.
# Poll the log for the "subscribe" confirmation instead of a fixed
# sleep so we don't race-publish before the subscriber is live.
for i in 1 2 3 4 5 6 7 8 9 10; do
  if grep -q '^subscribe' "$sub_log" 2>/dev/null; then
    break
  fi
  sleep 0.3
done
grep -q '^subscribe' "$sub_log" 2>/dev/null || fail "subscribe did not confirm within 3s"

cleanup() {
  if kill -0 "$sub_pid" 2>/dev/null; then
    kill "$sub_pid" 2>/dev/null || true
    wait "$sub_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT

# Create the draft campaign. budgets are generous enough that /start
# will satisfy the balance + budget_total>=budget_daily checks later
# (after 30-topup has credited 500000c to the advertiser).
create_body=$(cat <<'EOF'
{
  "name": "qa-harness-campaign",
  "billing_model": "cpm",
  "budget_total_cents": 1000000,
  "budget_daily_cents": 100000,
  "bid_cpm_cents": 150,
  "targeting": {}
}
EOF
)
create_resp=$(curl_json POST /api/v1/campaigns "$create_body" "$api_key")
create_body_out=$(assert_status 201 "$create_resp") || fail "POST /campaigns"

cid=$(json_field "$create_body_out" id)
[ -z "$cid" ] && fail "campaign create returned no id: $create_body_out"
[ "$cid" = "0" ] && fail "campaign create returned id=0: $create_body_out"
step_ok "campaign created id=${cid}"

# PUT update — rename + tweak budgets so we can verify pub/sub fires
# on this path too. Handler writes all four fields unconditionally.
update_body=$(cat <<EOF
{
  "name": "qa-harness-campaign-renamed",
  "bid_cpm_cents": 175,
  "budget_daily_cents": 120000,
  "targeting": {}
}
EOF
)
update_resp=$(curl_json PUT "/api/v1/campaigns/${cid}" "$update_body" "$api_key")
update_body_out=$(assert_status 200 "$update_resp") || fail "PUT /campaigns/${cid}"

status=$(json_field "$update_body_out" status)
[ "$status" = "updated" ] || fail "PUT /campaigns/${cid}: expected status=updated got=${status} body=${update_body_out}"
step_ok "campaign ${cid} updated"

# Wait briefly so the publish lands in the subscriber log, then stop
# the background subscriber before the grep check.
sleep 0.7
cleanup
trap - EXIT

# At this point sub_log should contain >=2 messages for our campaign:
# one for create (action=updated) and one for update (action=updated).
match_count=$(grep -c "\"campaign_id\":${cid}" "$sub_log" || true)
if [ "${match_count:-0}" -lt 2 ]; then
  log "sub_log contents:"
  cat "$sub_log" >&2 || true
  fail "expected >=2 campaign:updates messages for cid=${cid}, saw ${match_count}"
fi
step_ok "pub/sub saw ${match_count} messages for cid=${cid}"

save_state campaign_id "$cid"
step_ok "40-campaign done"
