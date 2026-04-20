$ErrorActionPreference = "Stop"

$root = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $root

if (-not $env:QA_POSTGRES_DSN) { $env:QA_POSTGRES_DSN = 'postgres://dsp:dsp_test_password@localhost:6432/dsp_test?sslmode=disable' }
if (-not $env:QA_REDIS_ADDR) { $env:QA_REDIS_ADDR = 'localhost:7380' }
if (-not $env:QA_REDIS_PASSWORD) { $env:QA_REDIS_PASSWORD = 'dsp_test_password' }
if (-not $env:QA_REDIS_DB) { $env:QA_REDIS_DB = '15' }
if (-not $env:QA_KAFKA_BROKERS) { $env:QA_KAFKA_BROKERS = 'localhost:10094' }
if (-not $env:QA_CLICKHOUSE_ADDR) { $env:QA_CLICKHOUSE_ADDR = 'localhost:10001' }
if (-not $env:QA_CLICKHOUSE_USER) { $env:QA_CLICKHOUSE_USER = 'default' }
if (-not $env:QA_CLICKHOUSE_PASSWORD) { $env:QA_CLICKHOUSE_PASSWORD = 'dsp_test_password' }

$packages = @(
  "./test/integration/..."
  "./internal/bidder/..."
  "./cmd/bidder/..."
  "./cmd/consumer/..."
  "./internal/reporting/..."
  "./internal/budget/..."
  "./internal/reconciliation/..."
)

go test -p 1 -tags integration -count=1 $packages
