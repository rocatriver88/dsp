"""
Shared fixtures and configuration for DSP frontend E2E tests.

Environment variables (defaults match docker-compose dev setup):
  DSP_FRONTEND_URL     - Next.js dev server  (default: http://localhost:14000)
  DSP_API_URL          - Advertiser API       (default: http://localhost:18181)
  DSP_ADMIN_API_URL    - Admin API            (default: http://localhost:18182)
  DSP_ADMIN_EMAIL      - Admin email          (default: admin@dsp.local)
  DSP_ADMIN_PASSWORD   - Admin password       (default: Admin123!)
  DSP_TENANT_EMAIL     - Tenant email         (default: alice@test.local)
  DSP_TENANT_PASSWORD  - Tenant password
"""

import os
import json
import urllib.request

# ---------------------------------------------------------------------------
# Environment
# ---------------------------------------------------------------------------
FRONTEND_URL = os.getenv("DSP_FRONTEND_URL", "http://localhost:14000")
API_URL = os.getenv("DSP_API_URL", "http://localhost:18181")
ADMIN_API_URL = os.getenv("DSP_ADMIN_API_URL", "http://localhost:18182")

# Admin credentials
ADMIN_EMAIL = os.getenv("DSP_ADMIN_EMAIL", "admin@dsp.local")
ADMIN_PASSWORD = os.getenv("DSP_ADMIN_PASSWORD", "Admin123!")

# Tenant credentials
TENANT_EMAIL = os.getenv("DSP_TENANT_EMAIL", "alice@test.local")
TENANT_PASSWORD = os.getenv("DSP_TENANT_PASSWORD", "2b11316c658a650b12826de8efdad4a7")

# Legacy — kept for backward compat with older test files
ADMIN_TOKEN = os.getenv("DSP_ADMIN_TOKEN", "admin-secret")
TEST_API_KEY = os.getenv("DSP_TEST_API_KEY", "")

HEADLESS = os.getenv("DSP_TEST_HEADLESS", "1") != "0"
SLOW_MO = int(os.getenv("DSP_TEST_SLOW_MO", "0"))


def launch_browser(playwright):
    """Launch a Chromium instance with project-standard options."""
    return playwright.chromium.launch(headless=HEADLESS, slow_mo=SLOW_MO)


def new_page(browser):
    """Create a page with a reasonable viewport."""
    context = browser.new_context(viewport={"width": 1280, "height": 800})
    return context.new_page()


def _api_login(email, password):
    """Call /api/v1/auth/login and return {access_token, refresh_token}."""
    body = json.dumps({"email": email, "password": password}).encode()
    req = urllib.request.Request(
        f"{API_URL}/api/v1/auth/login",
        data=body,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    resp = urllib.request.urlopen(req)
    return json.loads(resp.read())


def _mock_api_handler(route):
    """Fulfill API requests with empty 200 to prevent 401 clears."""
    route.fulfill(status=200, content_type="application/json", body="[]")


def tenant_login(page, api_key=None, mock_api=False, email=None, password=None):
    """Log in as a tenant.

    Supports three modes:
    1. email+password (preferred): calls API to get JWT, injects tokens
    2. api_key (legacy): injects API key into localStorage
    3. mock_api: mocks all API calls, uses fake auth
    """
    if mock_api:
        page.route("**/api/v1/**", _mock_api_handler)
        page.goto(FRONTEND_URL)
        page.wait_for_load_state("domcontentloaded")
        # Inject a fake API key so the gate opens
        page.evaluate('localStorage.setItem("dsp_api_key", "dsp_mock_key")')
        page.reload()
        page.wait_for_load_state("domcontentloaded")
        return

    if email and password:
        # JWT login
        tokens = _api_login(email, password)
        page.goto(FRONTEND_URL)
        page.wait_for_load_state("networkidle")
        page.evaluate(f'localStorage.setItem("dsp_access_token", "{tokens["access_token"]}")')
        page.evaluate(f'localStorage.setItem("dsp_refresh_token", "{tokens["refresh_token"]}")')
        page.reload()
        page.wait_for_load_state("networkidle")
        return

    if api_key:
        # Legacy API key login
        page.goto(FRONTEND_URL)
        page.wait_for_load_state("networkidle")
        page.evaluate(f'localStorage.setItem("dsp_api_key", "{api_key}")')
        page.reload()
        page.wait_for_load_state("networkidle")
        return

    # Default: use TENANT_EMAIL/PASSWORD
    tenant_login(page, email=TENANT_EMAIL, password=TENANT_PASSWORD)


def admin_login(page, token=None, email=None, password=None):
    """Log in as admin.

    Supports two modes:
    1. email+password (preferred): calls API to get JWT, injects tokens
    2. token (legacy): injects X-Admin-Token into localStorage
    """
    if email and password:
        tokens = _api_login(email, password)
        page.goto(f"{FRONTEND_URL}/admin")
        page.wait_for_load_state("networkidle")
        page.evaluate(f'localStorage.setItem("dsp_access_token", "{tokens["access_token"]}")')
        page.evaluate(f'localStorage.setItem("dsp_refresh_token", "{tokens["refresh_token"]}")')
        page.reload()
        page.wait_for_load_state("networkidle")
        return

    if token:
        # Legacy token login
        page.goto(f"{FRONTEND_URL}/admin")
        page.wait_for_load_state("networkidle")
        page.evaluate(f'localStorage.setItem("dsp_admin_token", "{token}")')
        page.reload()
        page.wait_for_load_state("networkidle")
        return

    # Default: use ADMIN_EMAIL/PASSWORD
    admin_login(page, email=ADMIN_EMAIL, password=ADMIN_PASSWORD)
