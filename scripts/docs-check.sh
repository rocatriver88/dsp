#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

echo "Regenerating OpenAPI + TS types..."
# Pre-download deps so swag's --parseDependency doesn't race go-module fetches and exit mid-stream on cold caches.
go mod download
# Fail loud if `go mod download` had to rewrite go.mod/go.sum — that means a dev forgot to commit module metadata.
if ! git diff --quiet -- go.mod go.sum; then
    echo "ERROR: 'go mod download' updated go.mod/go.sum — commit those changes and retry."
    git --no-pager diff -- go.mod go.sum
    exit 1
fi
# Keep full output so real failures surface in CI (chatty success output is acceptable cost).
swag init -g cmd/api/main.go -o docs/generated --parseDependency --parseInternal
(cd web && npm run generate:api)

if ! git diff --quiet -- docs/generated web/lib/api-types.ts; then
    echo "ERROR: generated contract is out of date."
    echo "Run: swag init -g cmd/api/main.go -o docs/generated --parseDependency --parseInternal"
    echo "Then: cd web && npm run generate:api"
    echo ""
    echo "Diff:"
    git diff -- docs/generated web/lib/api-types.ts
    exit 1
fi
echo "OK: generated contract is up to date."
