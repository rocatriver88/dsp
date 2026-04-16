#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

echo "Regenerating OpenAPI + TS types..."
swag init -g cmd/api/main.go -o docs/generated --parseDependency --parseInternal >/dev/null 2>&1
(cd web && npm run generate:api >/dev/null 2>&1)

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
