#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

export QA_POSTGRES_DSN="${QA_POSTGRES_DSN:-postgres://dsp:dsp_test_password@localhost:6432/dsp_test?sslmode=disable}"
export QA_REDIS_ADDR="${QA_REDIS_ADDR:-localhost:7380}"
export QA_REDIS_PASSWORD="${QA_REDIS_PASSWORD:-dsp_test_password}"
export QA_REDIS_DB="${QA_REDIS_DB:-15}"
export QA_KAFKA_BROKERS="${QA_KAFKA_BROKERS:-localhost:10094}"
export QA_CLICKHOUSE_ADDR="${QA_CLICKHOUSE_ADDR:-localhost:10001}"
export QA_CLICKHOUSE_USER="${QA_CLICKHOUSE_USER:-default}"
export QA_CLICKHOUSE_PASSWORD="${QA_CLICKHOUSE_PASSWORD:-dsp_test_password}"

go test -p 1 -tags integration -count=1 \
  ./test/integration/... \
  ./internal/bidder/... \
  ./cmd/bidder/... \
  ./cmd/consumer/... \
  ./internal/reporting/... \
  ./internal/budget/... \
  ./internal/reconciliation/...
