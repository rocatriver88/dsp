#!/usr/bin/env bash
# scripts/qa/60-reports.sh
# Seed a handful of bid_log rows for our campaign/creative straight
# into ClickHouse via the HTTP interface, then hit every reporting
# and export endpoint to confirm the happy-path 200s.
#
# Handler references (internal/handler/routes.go):
#   GET /api/v1/reports/campaign/{id}/stats
#   GET /api/v1/reports/campaign/{id}/hourly
#   GET /api/v1/reports/campaign/{id}/geo
#   GET /api/v1/reports/campaign/{id}/bids
#   GET /api/v1/reports/campaign/{id}/attribution
#   GET /api/v1/reports/campaign/{id}/simulate?bid_cpm_cents=150
#   GET /api/v1/reports/overview
#   GET /api/v1/export/campaign/{id}/stats  (text/csv)
#   GET /api/v1/export/campaign/{id}/bids   (text/csv)
set -euo pipefail
source "$(dirname "$0")/e2e-env.sh"
source "$(dirname "$0")/lib.sh"

step_start "60-reports"

advID=$(load_state advertiser_id)
api_key=$(load_state api_key)
cid=$(load_state campaign_id)
crid=$(load_state creative_id)
[ -z "$advID" ] && fail "advertiser_id state empty"
[ -z "$api_key" ] && fail "api_key state empty"
[ -z "$cid" ] && fail "campaign_id state empty"
[ -z "$crid" ] && fail "creative_id state empty"

# Unique per-run request_id prefix so repeat runs don't collide on
# the bid_log ReplacingMergeTree key (if ch ever dedups).
ts=$(date +%s)

sql="INSERT INTO bid_log (event_date, event_time, campaign_id, creative_id, advertiser_id, exchange_id, request_id, geo_country, device_os, device_id, bid_price_cents, clear_price_cents, charge_cents, event_type, loss_reason) VALUES
(today(), now(), ${cid}, ${crid}, ${advID}, 'qa-exchange', 'qa-req-1-${ts}', 'US', 'iOS',     'd1', 100, 80, 80, 'impression', ''),
(today(), now(), ${cid}, ${crid}, ${advID}, 'qa-exchange', 'qa-req-2-${ts}', 'US', 'iOS',     'd2', 100, 80, 80, 'impression', ''),
(today(), now(), ${cid}, ${crid}, ${advID}, 'qa-exchange', 'qa-req-3-${ts}', 'CN', 'Android', 'd3', 100, 80, 80, 'click', ''),
(today(), now(), ${cid}, ${crid}, ${advID}, 'qa-exchange', 'qa-req-4-${ts}', 'US', 'iOS',     'd4', 100, 80, 80, 'conversion', ''),
(today(), now(), ${cid}, ${crid}, ${advID}, 'qa-exchange', 'qa-req-5-${ts}', 'US', 'iOS',     'd5', 100, 80, 80, 'win', '')"

# ch_query posts raw SQL to the CH HTTP interface and returns whatever
# CH sent back. Successful INSERTs return empty body. Any ch error
# ("Code: 62...", "Cannot parse...") shows up in stdout as text.
ch_out=$(ch_query "$sql" 2>&1 || true)
if [ -n "$ch_out" ]; then
  log "clickhouse response: $ch_out"
  # A non-empty body that looks like a CH error should fail loudly.
  if printf '%s' "$ch_out" | grep -qE 'Code:|Exception|DB::|Cannot'; then
    fail "bid_log seed rejected by clickhouse"
  fi
fi
step_ok "bid_log seeded (5 rows, ts=${ts})"

# Hit each JSON report endpoint and assert 200. The Go e2e tests in
# P2.6 already cover the body shape — we just want the smoke-test
# signal here.
for path in \
  "/api/v1/reports/campaign/${cid}/stats" \
  "/api/v1/reports/campaign/${cid}/hourly" \
  "/api/v1/reports/campaign/${cid}/geo" \
  "/api/v1/reports/campaign/${cid}/bids" \
  "/api/v1/reports/campaign/${cid}/attribution" \
  "/api/v1/reports/campaign/${cid}/simulate?bid_cpm_cents=150" \
  "/api/v1/reports/overview"
do
  resp=$(curl_json GET "$path" "" "$api_key")
  assert_status 200 "$resp" >/dev/null || fail "GET $path"
  step_ok "GET $path -> 200"
done

# CSV exports. assert_status echoes the body so we can do a cheap
# smoke check (CSV always has at least one comma in the header row).
for path in \
  "/api/v1/export/campaign/${cid}/stats" \
  "/api/v1/export/campaign/${cid}/bids"
do
  resp=$(curl_json GET "$path" "" "$api_key")
  body=$(assert_status 200 "$resp") || fail "GET $path"
  if ! printf '%s' "$body" | grep -q ','; then
    log "csv body (first 200 bytes): $(printf '%s' "$body" | head -c 200)"
    fail "CSV export has no commas: $path"
  fi
  step_ok "GET $path -> 200 (csv)"
done

step_ok "60-reports done"
