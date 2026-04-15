#!/usr/bin/env bash
# scripts/qa/50-creative.sh
# Attach a creative to the draft campaign created in 40-campaign.sh,
# then transition the campaign to active. Verifies that every write
# (create creative, update creative, start campaign) publishes on
# campaign:updates.
#
# Handler references (internal/handler/campaign.go):
#   POST /api/v1/creatives            body: {campaign_id, name, ad_type, format, size,
#                                             ad_markup, destination_url}
#                                     resp: 201 {id, status:"approved"} (ENV != production)
#                                     pub:  campaign:updates action="updated"
#   PUT  /api/v1/creatives/{id}       body: {name, ad_type, format, size, ad_markup, destination_url}
#                                     resp: 200 {status:"updated"}
#                                     pub:  campaign:updates action="updated"
#   POST /api/v1/campaigns/{id}/start body: (none)
#                                     resp: 200 {status:"active"}
#                                     pub:  campaign:updates action="activated"
set -euo pipefail
source "$(dirname "$0")/e2e-env.sh"
source "$(dirname "$0")/lib.sh"

step_start "50-creative"

advID=$(load_state advertiser_id)
api_key=$(load_state api_key)
cid=$(load_state campaign_id)
[ -z "$advID" ] && fail "advertiser_id state empty"
[ -z "$api_key" ] && fail "api_key state empty"
[ -z "$cid" ] && fail "campaign_id state empty (run 40-campaign.sh first)"

sub_log="${QA_STATE_DIR}/updates-50-creative.log"
: > "$sub_log"

# One long-running subscriber covers all three write operations below.
# Poll the log for the "subscribe" confirmation instead of a fixed sleep
# so docker compose exec container start latency doesn't race-publish
# before the subscriber is actually listening.
redis_cmd SUBSCRIBE campaign:updates > "$sub_log" 2>&1 &
sub_pid=$!
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

# Create a banner creative. ad_type=banner + format=banner is the
# "safe combo" known to satisfy the schema CHECK constraints in dev.
create_body=$(cat <<EOF
{
  "campaign_id": ${cid},
  "name": "qa-creative-1",
  "ad_type": "banner",
  "format": "banner",
  "size": "300x250",
  "ad_markup": "<a href=\"https://example.com/qa\">qa ad</a>",
  "destination_url": "https://example.com/qa"
}
EOF
)
create_resp=$(curl_json POST /api/v1/creatives "$create_body" "$api_key")
create_body_out=$(assert_status 201 "$create_resp") || fail "POST /creatives"

crid=$(json_field "$create_body_out" id)
cstatus=$(json_field "$create_body_out" status)
[ -z "$crid" ] && fail "creative create returned no id: $create_body_out"
[ "$crid" = "0" ] && fail "creative create returned id=0: $create_body_out"
step_ok "creative created id=${crid} status=${cstatus}"

# Update the creative to exercise the update path + pub/sub.
update_body=$(cat <<'EOF'
{
  "name": "qa-creative-1-renamed",
  "ad_type": "banner",
  "format": "banner",
  "size": "300x250",
  "ad_markup": "<a href=\"https://example.com/qa2\">qa ad v2</a>",
  "destination_url": "https://example.com/qa2"
}
EOF
)
update_resp=$(curl_json PUT "/api/v1/creatives/${crid}" "$update_body" "$api_key")
update_body_out=$(assert_status 200 "$update_resp") || fail "PUT /creatives/${crid}"
cstatus2=$(json_field "$update_body_out" status)
[ "$cstatus2" = "updated" ] || fail "PUT /creatives/${crid}: expected status=updated got=${cstatus2}"
step_ok "creative ${crid} updated"

# Now the campaign has >=1 approved creative, a generous budget, and
# the advertiser has a post-topup balance that clears budget_daily.
# Transition to active. Handler tolerates any body; send {}.
start_resp=$(curl_json POST "/api/v1/campaigns/${cid}/start" "{}" "$api_key")
start_body_out=$(assert_status 200 "$start_resp") || fail "POST /campaigns/${cid}/start"
sstatus=$(json_field "$start_body_out" status)
[ "$sstatus" = "active" ] || fail "start campaign ${cid}: expected status=active got=${sstatus}"
step_ok "campaign ${cid} started (status=${sstatus})"

# Wait for the final publish to land, then stop the subscriber.
sleep 0.7
cleanup
trap - EXIT

# Expected messages on this channel for THIS campaign:
#   1. creative create  -> action "updated"
#   2. creative update  -> action "updated"
#   3. campaign start   -> action "activated"
# so sub_log should mention campaign_id at least 3 times and
# at least one "activated".
match_count=$(grep -c "\"campaign_id\":${cid}" "$sub_log" || true)
if [ "${match_count:-0}" -lt 3 ]; then
  log "sub_log contents:"
  cat "$sub_log" >&2 || true
  fail "expected >=3 campaign:updates messages for cid=${cid}, saw ${match_count}"
fi
if ! grep -q "\"action\":\"activated\"" "$sub_log"; then
  log "sub_log contents:"
  cat "$sub_log" >&2 || true
  fail "never saw action=activated in campaign:updates"
fi
step_ok "pub/sub saw ${match_count} messages for cid=${cid} incl. activated"

save_state creative_id "$crid"
step_ok "50-creative done"
