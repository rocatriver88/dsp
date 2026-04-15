#!/usr/bin/env bash
# scripts/qa/e2e-env.sh
# Source this before running any other script in this directory.
# Defaults match the biz worktree's +11000 port offset and the
# dsp_dev_password baked into docker-compose.yml.

export DSP_E2E_PG_DSN="${DSP_E2E_PG_DSN:-postgres://dsp:dsp_dev_password@localhost:16432/dsp?sslmode=disable}"
export DSP_E2E_REDIS_HOST="${DSP_E2E_REDIS_HOST:-localhost}"
export DSP_E2E_REDIS_PORT="${DSP_E2E_REDIS_PORT:-17380}"
export DSP_E2E_REDIS_PASSWORD="${DSP_E2E_REDIS_PASSWORD:-dsp_dev_password}"
export DSP_E2E_CH_HTTP="${DSP_E2E_CH_HTTP:-http://localhost:19124}"
export DSP_E2E_CH_USER="${DSP_E2E_CH_USER:-default}"
export DSP_E2E_CH_PASSWORD="${DSP_E2E_CH_PASSWORD:-dsp_dev_password}"
export DSP_E2E_ADMIN_TOKEN="${DSP_E2E_ADMIN_TOKEN:-admin-secret}"
export DSP_E2E_API="${DSP_E2E_API:-http://localhost:19181}"
export DSP_E2E_ADMIN_API="${DSP_E2E_ADMIN_API:-http://localhost:19182}"
