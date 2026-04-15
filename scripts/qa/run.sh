#!/usr/bin/env bash
# scripts/qa/run.sh
# Main harness entry. Sources env + lib, then runs each numbered
# step script in order. Fails fast on the first step that errors.
set -euo pipefail
cd "$(dirname "$0")"
source ./e2e-env.sh
source ./lib.sh
cd ../..  # back to repo root so relative paths resolve

REPORT_DIR="docs/qa"
mkdir -p "$REPORT_DIR"

log "== biz QA harness start =="
log "state dir: $QA_STATE_DIR"
log "api:       $DSP_E2E_API"
log "admin api: $DSP_E2E_ADMIN_API"

# Step list. P3.1 only ships 00-bootstrap.sh; 10-99 are added by
# P3.2 / P3.3. Missing scripts are skipped with a log so the harness
# can run incrementally as tasks land.
STEPS=(
  00-bootstrap.sh
  10-invite-admin.sh
  20-register.sh
  30-topup.sh
  40-campaign.sh
  50-creative.sh
  60-reports.sh
  70-admin-review.sh
  99-teardown.sh
)

failed_steps=()
for step in "${STEPS[@]}"; do
  script="scripts/qa/$step"
  if [ ! -f "$script" ]; then
    log ">> $step SKIP (not yet implemented)"
    continue
  fi
  log ">> $step"
  if ! bash "$script"; then
    log "!! $step FAILED"
    failed_steps+=("$step")
    break
  fi
done

if [ "${#failed_steps[@]}" -eq 0 ]; then
  log "== ALL STEPS PASSED =="
  exit 0
else
  log "== FAILED: ${failed_steps[*]} =="
  exit 1
fi
