#!/usr/bin/env bash
# scripts/qa/99-teardown.sh
# Print a human-readable summary of what the harness produced.
# Intentionally does NOT delete the advertiser, campaign, creative,
# bid_log rows, or $QA_STATE_DIR — those are left behind so humans
# can inspect after a run. docker volumes are untouched.
set -euo pipefail
source "$(dirname "$0")/e2e-env.sh"
source "$(dirname "$0")/lib.sh"

step_start "99-teardown"

# Best-effort state reads. Use cat directly (not load_state) because
# we want the summary to survive a partial run — load_state fails
# hard on missing files and we'd rather print "?" than abort.
read_state() {
  local f="$QA_STATE_DIR/$1"
  if [ -f "$f" ]; then
    cat "$f"
  else
    printf '?'
  fi
}

advID=$(read_state advertiser_id)
email=$(read_state contact_email)
api_key=$(read_state api_key)
topup=$(read_state topup_amount)
cid=$(read_state campaign_id)
crid=$(read_state creative_id)
balance_after=$(read_state balance_after_topup)

# Truncate the api_key for the printout — it's the caller credential
# and we don't need the full 64 hex chars in logs.
api_key_short="${api_key:0:12}"
if [ "${#api_key}" -gt 12 ]; then
  api_key_short="${api_key_short}..."
fi

cat <<EOF
== harness summary ==
  state dir:            ${QA_STATE_DIR}
  api:                  ${DSP_E2E_API}
  admin api:            ${DSP_E2E_ADMIN_API}
  advertiser:           id=${advID} email=${email}
  api_key:              ${api_key_short}
  topup_amount_cents:   ${topup}
  balance_after_topup:  ${balance_after}
  campaign_id:          ${cid}
  creative_id:          ${crid}
EOF

# List every state file so a human reading the harness output knows
# exactly what to find in $QA_STATE_DIR for post-mortem inspection.
if [ -d "$QA_STATE_DIR" ]; then
  log "state files:"
  ls -1 "$QA_STATE_DIR" | while read -r f; do
    log "  - $f"
  done
fi

log "== harness complete =="
