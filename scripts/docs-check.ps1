$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $root

function Invoke-Native {
  param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$Command
  )

  $exe = $Command[0]
  $args = @()
  if ($Command.Length -gt 1) {
    $args = $Command[1..($Command.Length - 1)]
  }
  & $exe @args
  if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
  }
}

Write-Host "Regenerating OpenAPI + TS types..."

Invoke-Native go mod download

$moduleDiff = git diff --name-only -- go.mod go.sum
if ($moduleDiff) {
  Write-Host "ERROR: 'go mod download' updated go.mod/go.sum — commit those changes and retry."
  git --no-pager diff -- go.mod go.sum
  exit 1
}

Invoke-Native swag init -g cmd/api/main.go -o docs/generated --parseDependency --parseInternal
Push-Location web
try {
  Invoke-Native npm run generate:api
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
