"""
E2E tests for login flows (tenant + admin).

Covers:
  - Tenant login page renders correctly
  - Email/password login form works
  - Valid credentials grant access to main app
  - Admin login page renders correctly
  - Admin email/password auth flow
  - Logout clears state
"""

from playwright.sync_api import sync_playwright, expect
from conftest import (
    FRONTEND_URL, ADMIN_EMAIL, ADMIN_PASSWORD,
    TENANT_EMAIL, TENANT_PASSWORD,
    launch_browser, new_page, tenant_login, admin_login,
)


def test_tenant_login_page_renders():
    """Login page shows email/password inputs and login button."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        page.goto(FRONTEND_URL)
        page.wait_for_load_state("networkidle")

        # Heading
        heading = page.locator("h2")
        expect(heading).to_have_text("DSP Platform")

        # Email + password inputs
        email_input = page.locator('input[type="email"]')
        expect(email_input).to_be_visible()
        password_input = page.locator('input[type="password"]')
        expect(password_input).to_be_visible()

        # Login button - should be disabled initially
        login_btn = page.locator("button", has_text="登录").first
        expect(login_btn).to_be_visible()

        browser.close()


def test_tenant_login_button_enables_on_input():
    """Login button enables when both email and password are filled."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        page.goto(FRONTEND_URL)
        page.wait_for_load_state("networkidle")

        email_input = page.locator('input[type="email"]')
        password_input = page.locator('input[type="password"]')
        login_btn = page.locator("button", has_text="登录").first

        # Fill both fields
        email_input.fill("test@example.com")
        password_input.fill("password123")

        # Button should be enabled
        expect(login_btn).to_be_enabled()

        browser.close()


def test_tenant_login_with_credentials():
    """Valid email+password login shows the main app."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        tenant_login(page, email=TENANT_EMAIL, password=TENANT_PASSWORD)

        # After login, main content should be visible
        expect(page.locator("#main-content")).to_be_visible(timeout=5000)

        browser.close()


def test_tenant_logout():
    """Logging out returns to the login screen."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        tenant_login(page, email=TENANT_EMAIL, password=TENANT_PASSWORD)

        # Clear tokens to simulate logout
        page.evaluate("""
            localStorage.removeItem("dsp_access_token");
            localStorage.removeItem("dsp_refresh_token");
            localStorage.removeItem("dsp_api_key");
        """)
        page.reload()
        page.wait_for_load_state("networkidle")

        # Login form should be back
        expect(page.locator("h2", has_text="DSP Platform")).to_be_visible()

        browser.close()


def test_admin_login_page_renders():
    """Admin login page shows email/password inputs."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        page.goto(f"{FRONTEND_URL}/admin")
        page.wait_for_load_state("networkidle")

        # Should show admin login heading
        heading = page.locator("h2")
        expect(heading).to_be_visible()

        # Email and password inputs
        expect(page.locator('input[type="email"]')).to_be_visible()
        expect(page.locator('input[type="password"]')).to_be_visible()

        browser.close()


def test_admin_login_with_credentials():
    """Valid admin credentials grant access to the admin console."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        admin_login(page, email=ADMIN_EMAIL, password=ADMIN_PASSWORD)

        # After login, admin sidebar should be visible
        sidebar = page.locator('nav[aria-label="管理员导航"]')
        expect(sidebar).to_be_visible()

        browser.close()


def test_admin_logout():
    """Admin logout returns to the admin login screen."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        admin_login(page, email=ADMIN_EMAIL, password=ADMIN_PASSWORD)

        # Clear tokens
        page.evaluate("""
            localStorage.removeItem("dsp_access_token");
            localStorage.removeItem("dsp_refresh_token");
        """)
        page.reload()
        page.wait_for_load_state("networkidle")

        # Admin login form should be back
        expect(page.locator('input[type="email"]')).to_be_visible()

        browser.close()


if __name__ == "__main__":
    print("Running login tests...")
    tests = [
        test_tenant_login_page_renders,
        test_tenant_login_button_enables_on_input,
        test_tenant_login_with_credentials,
        test_tenant_logout,
        test_admin_login_page_renders,
        test_admin_login_with_credentials,
        test_admin_logout,
    ]
    passed = 0
    failed = 0
    for t in tests:
        try:
            t()
            print(f"  PASS: {t.__name__}")
            passed += 1
        except Exception as e:
            print(f"  FAIL: {t.__name__}: {e}")
            failed += 1
    print(f"\n{passed} passed, {failed} failed")
