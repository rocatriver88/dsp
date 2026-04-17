#!/usr/bin/env bash
# test/e2e/run.sh
# Run all E2E frontend tests against a live dev environment.
#
# Prerequisites:
#   - Python 3.12+ with playwright installed
#   - Chromium browser installed (python -m playwright install chromium)
#   - Backend services running (docker compose up)
#   - Frontend dev server running (cd web && npm run dev)
#
# Usage:
#   bash test/e2e/run.sh              # run all tests
#   bash test/e2e/run.sh login        # run only login tests
#   bash test/e2e/run.sh screenshots  # capture screenshots only
#
# Environment (all optional, defaults match dev setup):
#   DSP_FRONTEND_URL=http://localhost:4000
#   DSP_API_URL=http://localhost:8181
#   DSP_ADMIN_API_URL=http://localhost:8182
#   DSP_ADMIN_TOKEN=admin-secret
#   DSP_TEST_API_KEY=dsp_...          # skip if tests create their own
#   DSP_TEST_HEADLESS=1               # set to 0 to see the browser
#   DSP_TEST_SLOW_MO=0                # ms between actions (for debugging)
set -euo pipefail
cd "$(dirname "$0")"

suite="${1:-all}"

run_test() {
    local name="$1"
    local script="$2"
    echo ""
    echo "========================================="
    echo "  $name"
    echo "========================================="
    python "$script"
}

case "$suite" in
    login)
        run_test "Login Tests" test_login.py
        ;;
    navigation|nav)
        run_test "Navigation Tests" test_navigation.py
        ;;
    design)
        run_test "Design Compliance Tests" test_design_compliance.py
        ;;
    admin)
        run_test "Admin Workflow Tests" test_admin_workflows.py
        ;;
    tenant)
        run_test "Tenant Workflow Tests" test_tenant_workflows.py
        ;;
    screenshots|ss)
        run_test "Screenshot Capture" test_screenshots.py
        ;;
    all)
        run_test "Login Tests" test_login.py
        run_test "Navigation Tests" test_navigation.py
        run_test "Design Compliance Tests" test_design_compliance.py
        run_test "Admin Workflow Tests" test_admin_workflows.py
        run_test "Tenant Workflow Tests" test_tenant_workflows.py
        run_test "Screenshot Capture" test_screenshots.py
        echo ""
        echo "========================================="
        echo "  ALL SUITES COMPLETE"
        echo "========================================="
        ;;
    *)
        echo "Unknown suite: $suite"
        echo "Usage: $0 [login|nav|design|admin|tenant|screenshots|all]"
        exit 1
        ;;
esac
