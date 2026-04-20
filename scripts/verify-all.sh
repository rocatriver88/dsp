#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_DIR"

SKIP_AUTOPILOT="${SKIP_AUTOPILOT:-0}"

echo "==> Go short tests"
go test ./... -short -count=1

echo "==> Handler e2e tests"
go test -tags e2e ./internal/handler/... -count=1

echo "==> Integration tests (serial)"
bash scripts/test-integration-serial.sh

echo "==> Frontend lint"
(cd web && npm run lint)

echo "==> Frontend build"
(cd web && npm run build)

echo "==> Generated contract check"
if command -v pwsh >/dev/null 2>&1; then
  pwsh -File ./scripts/docs-check.ps1
elif command -v powershell >/dev/null 2>&1; then
  powershell -ExecutionPolicy Bypass -File ./scripts/docs-check.ps1
else
  bash ./scripts/docs-check.sh
fi

if [[ "$SKIP_AUTOPILOT" == "1" ]]; then
  echo "==> Skipping autopilot verify (SKIP_AUTOPILOT=1)"
  exit 0
fi

echo "==> Autopilot verify"
go run ./cmd/autopilot verify
