"""
Screenshot capture for all key pages.

Not assertion-heavy — primarily generates visual evidence for manual
review or diff-based regression checks. Screenshots are saved to
/tmp/dsp_e2e_screenshots/.
"""

import os
from playwright.sync_api import sync_playwright
from conftest import (
    FRONTEND_URL, ADMIN_TOKEN, TEST_API_KEY,
    launch_browser, new_page, tenant_login, admin_login,
)

SCREENSHOT_DIR = os.environ.get("DSP_SCREENSHOT_DIR", "/tmp/dsp_e2e_screenshots")


def capture_all():
    """Capture screenshots of every page in both tenant and admin contexts."""
    os.makedirs(SCREENSHOT_DIR, exist_ok=True)

    api_key = TEST_API_KEY or "dsp_e2e_test_key"

    with sync_playwright() as p:
        browser = launch_browser(p)

        # --- Login page (unauthenticated) ---
        page = new_page(browser)
        page.goto(FRONTEND_URL)
        page.wait_for_load_state("networkidle")
        page.screenshot(path=f"{SCREENSHOT_DIR}/00_login.png", full_page=True)

        # --- Admin login page ---
        page.goto(f"{FRONTEND_URL}/admin")
        page.wait_for_load_state("networkidle")
        page.screenshot(path=f"{SCREENSHOT_DIR}/01_admin_login.png", full_page=True)
        page.close()

        # --- Tenant pages ---
        tenant_routes = [
            ("10_dashboard", "/"),
            ("11_campaigns", "/campaigns"),
            ("12_analytics", "/analytics"),
            ("13_billing", "/billing"),
            ("14_reports", "/reports"),
        ]
        page = new_page(browser)
        mock = not bool(TEST_API_KEY)
        tenant_login(page, api_key, mock_api=mock)

        wait_state = "domcontentloaded" if mock else "networkidle"
        for name, path in tenant_routes:
            page.goto(f"{FRONTEND_URL}{path}")
            page.wait_for_load_state(wait_state)
            # Extra wait for dynamic content (charts, tables)
            page.wait_for_timeout(1000)
            page.screenshot(path=f"{SCREENSHOT_DIR}/{name}.png", full_page=True)
        page.close()

        # --- Admin pages ---
        admin_routes = [
            ("20_admin_overview", "/admin"),
            ("21_admin_agencies", "/admin/agencies"),
            ("22_admin_creatives", "/admin/creatives"),
            ("23_admin_invites", "/admin/invites"),
            ("24_admin_audit", "/admin/audit"),
        ]
        page = new_page(browser)
        admin_login(page)

        for name, path in admin_routes:
            page.goto(f"{FRONTEND_URL}{path}")
            page.wait_for_load_state("networkidle")
            page.wait_for_timeout(1000)
            page.screenshot(path=f"{SCREENSHOT_DIR}/{name}.png", full_page=True)
        page.close()

        browser.close()

    print(f"Screenshots saved to {SCREENSHOT_DIR}/")
    for f in sorted(os.listdir(SCREENSHOT_DIR)):
        if f.endswith(".png"):
            print(f"  {f}")


if __name__ == "__main__":
    capture_all()
