$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $root

Write-Host "Regenerating OpenAPI + TS types..."

go mod download
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$moduleDiff = git -c core.autocrlf=false diff --name-only -- go.mod go.sum 2>$null
if ($moduleDiff) {
  Write-Host "ERROR: 'go mod download' updated go.mod/go.sum — commit those changes and retry."
  git --no-pager diff -- go.mod go.sum
  exit 1
}

$swagOutput = & swag init -g cmd/api/main.go -o docs/generated --parseDependency --parseInternal 2>&1
if ($LASTEXITCODE -ne 0) {
  $swagOutput | Write-Host
  exit $LASTEXITCODE
}
Push-Location web
try {
  npm run generate:api
  if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally {
  Pop-Location
}

$generatedDiff = git -c core.autocrlf=false diff --name-only -- docs/generated web/lib/api-types.ts 2>$null
if ($generatedDiff) {
  Write-Host "ERROR: generated contract is out of date."
  Write-Host "Run: swag init -g cmd/api/main.go -o docs/generated --parseDependency --parseInternal"
  Write-Host "Then: cd web && npm run generate:api"
  Write-Host ""
  Write-Host "Diff:"
  git -c core.autocrlf=false diff -- docs/generated web/lib/api-types.ts
  exit 1
}

Write-Host "OK: generated contract is up to date."
