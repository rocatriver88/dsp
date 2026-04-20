#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

echo "Regenerating OpenAPI + TS types..."
# Pre-download deps so swag's --parseDependency doesn't race go-module fetches and exit mid-stream on cold caches.
go mod download
# Fail loud if `go mod download` had to rewrite go.mod/go.sum — that means a dev forgot to commit module metadata.
if ! git -c core.autocrlf=false diff --quiet -- go.mod go.sum 2>/dev/null; then
    echo "ERROR: 'go mod download' updated go.mod/go.sum — commit those changes and retry."
    git --no-pager diff -- go.mod go.sum
    exit 1
fi
# Swag emits noisy warnings on Windows / newer Go toolchains; hide them on
# success but still print the full output if generation actually fails.
swag_output="$(mktemp)"
if ! swag init -g cmd/api/main.go -o docs/generated --parseDependency --parseInternal >"$swag_output" 2>&1; then
    cat "$swag_output"
    rm -f "$swag_output"
    exit 1
fi
rm -f "$swag_output"
(cd web && npm run generate:api)

if ! git -c core.autocrlf=false diff --quiet -- docs/generated web/lib/api-types.ts 2>/dev/null; then
    echo "ERROR: generated contract is out of date."
    echo "Run: swag init -g cmd/api/main.go -o docs/generated --parseDependency --parseInternal"
    echo "Then: cd web && npm run generate:api"
    echo ""
    echo "Diff:"
    git -c core.autocrlf=false diff -- docs/generated web/lib/api-types.ts
    exit 1
fi
echo "OK: generated contract is up to date."
