#!/usr/bin/env bash
# scripts/qa/00-bootstrap.sh
# Wait for the biz docker stack to be fully ready, including the api
# service. Fails loudly on timeout so subsequent scripts don't race.
set -euo pipefail
source "$(dirname "$0")/e2e-env.sh"
source "$(dirname "$0")/lib.sh"

step_start "00-bootstrap"

# 1. Core tooling check. curl is mandatory. For JSON parsing we accept
#    either jq (preferred) or python / python3 (fallback via json_field
#    helpers in lib.sh). redis-cli is optional — lib.sh falls back to
#    docker compose exec when absent. clickhouse-client is NOT required
#    -- we use the ClickHouse HTTP interface directly.
have_tool curl || fail "missing required tool: curl"
if ! have_tool jq && ! have_tool python && ! have_tool python3; then
  fail "missing JSON reader: install jq or python (used by scripts/qa/lib.sh:json_field)"
fi
have_tool jq || log "warn: jq not on host -- json_field will fall back to python"
have_tool redis-cli || log "warn: redis-cli not on host -- will use docker compose exec fallback"
step_ok "tooling check"

# 2. Wait for public api /health.
log "waiting for public api at ${DSP_E2E_API}/health ..."
elapsed=0
max_wait=60
while [ "$elapsed" -lt "$max_wait" ]; do
  if curl -sfo /dev/null "${DSP_E2E_API}/health"; then
    step_ok "public api ready after ${elapsed}s"
    break
  fi
  sleep 2
  elapsed=$((elapsed + 2))
done
[ "$elapsed" -ge "$max_wait" ] && fail "public api ${DSP_E2E_API}/health not reachable after ${max_wait}s -- run: docker compose up -d api"

# 3. Wait for internal api /health.
log "waiting for internal api at ${DSP_E2E_ADMIN_API}/health ..."
elapsed=0
while [ "$elapsed" -lt "$max_wait" ]; do
  if curl -sfo /dev/null "${DSP_E2E_ADMIN_API}/health"; then
    step_ok "internal api ready after ${elapsed}s"
    break
  fi
  sleep 2
  elapsed=$((elapsed + 2))
done
[ "$elapsed" -ge "$max_wait" ] && fail "internal api ${DSP_E2E_ADMIN_API}/health not reachable after ${max_wait}s -- run: docker compose up -d api"

# 4. Redis sanity (PING). Uses redis_cmd which handles missing host client.
if redis_cmd PING >/dev/null 2>&1; then
  step_ok "redis PING ok"
else
  fail "redis unreachable at ${DSP_E2E_REDIS_HOST}:${DSP_E2E_REDIS_PORT} -- run: docker compose up -d redis"
fi

# 5. ClickHouse sanity. The bid_log table must exist or 60-reports.sh
#    will fail with a cryptic error later.
ch_ping=$(ch_query "SELECT 1" 2>&1 || true)
if [ "$ch_ping" != "1" ]; then
  fail "clickhouse unreachable at ${DSP_E2E_CH_HTTP} (got: $ch_ping) -- run: docker compose up -d clickhouse"
fi
ch_table=$(ch_query "EXISTS TABLE bid_log" 2>&1 || true)
if [ "$ch_table" != "1" ]; then
  fail "clickhouse bid_log table missing (got: $ch_table) -- check migrations/002_clickhouse.sql"
fi
step_ok "clickhouse ready (bid_log exists)"

# 6. Postgres sanity. The advertisers table must exist.
pg_ok=$(docker compose exec -T postgres psql -U dsp -d dsp -tA \
  -c "SELECT 1 FROM information_schema.tables WHERE table_name='advertisers'" 2>&1 || true)
if [ "$pg_ok" != "1" ]; then
  fail "postgres schema missing advertisers table (got: $pg_ok) -- run migrations"
fi
step_ok "postgres ready (advertisers exists)"

step_ok "bootstrap complete; state dir = ${QA_STATE_DIR}"
