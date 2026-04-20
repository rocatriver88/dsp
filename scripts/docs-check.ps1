$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $root

Write-Host "Regenerating OpenAPI + TS types..."

go mod download
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$moduleDiff = git diff --name-only -- go.mod go.sum
if ($moduleDiff) {
  Write-Host "ERROR: 'go mod download' updated go.mod/go.sum — commit those changes and retry."
  git --no-pager diff -- go.mod go.sum
  exit 1
}

swag init -g cmd/api/main.go -o docs/generated --parseDependency --parseInternal
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Push-Location web
try {
  npm run generate:api
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally {
  Pop-Location
}

$generatedDiff = git diff --name-only -- docs/generated web/lib/api-types.ts
if ($generatedDiff) {
  Write-Host "ERROR: generated contract is out of date."
  Write-Host "Run: swag init -g cmd/api/main.go -o docs/generated --parseDependency --parseInternal"
  Write-Host "Then: cd web && npm run generate:api"
  Write-Host ""
  Write-Host "Diff:"
  git diff -- docs/generated web/lib/api-types.ts
  exit 1
}

Write-Host "OK: generated contract is up to date."
