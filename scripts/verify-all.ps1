param(
  [switch]$SkipAutopilot
)

$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $root

Write-Host "==> Go short tests"
go test ./... -short -count=1

Write-Host "==> Handler e2e tests"
go test -tags e2e ./internal/handler/... -count=1

Write-Host "==> Integration tests (serial)"
powershell -ExecutionPolicy Bypass -File .\scripts\test-integration-serial.ps1

Write-Host "==> Frontend lint"
Push-Location web
try {
  npm run lint
  Write-Host "==> Frontend build"
  npm run build
} finally {
  Pop-Location
}

Write-Host "==> Generated contract check"
powershell -ExecutionPolicy Bypass -File .\scripts\docs-check.ps1

if ($SkipAutopilot -or $env:SKIP_AUTOPILOT -eq "1") {
  Write-Host "==> Skipping autopilot verify"
  exit 0
}

Write-Host "==> Autopilot verify"
go run ./cmd/autopilot verify
