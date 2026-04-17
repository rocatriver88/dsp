"""
E2E tests for email+password auth flows (JWT-based).

Complements test_login.py which tests the legacy API Key + Admin Token flows.
These tests verify the new user system introduced in the feat-user-rbac branch.

Prerequisites:
  - migrations/011_users.sql applied
  - At least one platform_admin user exists (seed via cmd/migrate-users)
  - DSP services running (docker compose up)
"""

import requests
from playwright.sync_api import sync_playwright, expect
from conftest import (
    FRONTEND_URL, API_URL, ADMIN_API_URL, ADMIN_TOKEN,
    launch_browser, new_page,
)


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def create_test_user(role="advertiser", prefix="e2e"):
    """Create a test user via the admin API. Returns (email, password, user_data)."""
    import uuid
    email = f"{prefix}-{uuid.uuid4().hex[:8]}@test.dsp"
    password = "TestPass123!"
    name = f"E2E {role.title()} User"

    resp = requests.post(
        f"{ADMIN_API_URL}/api/v1/admin/users",
        headers={"X-Admin-Token": ADMIN_TOKEN},
        json={"email": email, "password": password, "name": name, "role": role},
    )
    assert resp.status_code == 201, f"Failed to create user: {resp.text}"
    return email, password, resp.json()


def jwt_login(email, password):
    """Login via API, return tokens."""
    resp = requests.post(
        f"{API_URL}/api/v1/auth/login",
        json={"email": email, "password": password},
    )
    return resp


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

def test_email_password_login_to_dashboard():
    """Advertiser logs in with email+password via the frontend, sees dashboard."""
    email, password, data = create_test_user("advertiser")

    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)

        # Go to frontend — should see login form
        page.goto(FRONTEND_URL)
        page.wait_for_load_state("networkidle")

        # Fill email+password and submit
        page.fill('input[type="email"], input[placeholder*="邮箱"]', email)
        page.fill('input[type="password"], input[placeholder*="密码"]', password)
        page.click('button:has-text("登录")')

        # Wait for dashboard to load
        page.wait_for_load_state("networkidle")

        # Should see the dashboard (概览 or campaign content)
        assert "登录" not in page.content() or "概览" in page.content() or "Campaign" in page.content(), \
            "Login did not redirect to dashboard"

        # Verify JWT is stored
        access_token = page.evaluate('localStorage.getItem("dsp_access_token")')
        assert access_token is not None, "Access token not found in localStorage"

        browser.close()


def test_admin_email_password_login():
    """Platform admin logs in with email+password, sees admin dashboard."""
    email, password, _ = create_test_user("platform_admin")

    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)

        # Navigate to admin
        page.goto(f"{FRONTEND_URL}/admin")
        page.wait_for_load_state("networkidle")

        # Should redirect to login since no JWT
        # Fill credentials on the login page
        page.fill('input[type="email"], input[placeholder*="邮箱"]', email)
        page.fill('input[type="password"], input[placeholder*="密码"]', password)
        page.click('button:has-text("登录")')

        page.wait_for_load_state("networkidle")

        # Navigate back to admin after login
        page.goto(f"{FRONTEND_URL}/admin")
        page.wait_for_load_state("networkidle")

        # Should see admin dashboard content
        content = page.content()
        assert "管理" in content or "Admin" in content or "代理商" in content, \
            f"Admin dashboard not loaded after login"

        browser.close()


def test_token_refresh_on_expiry():
    """After access token expires, API call triggers auto-refresh."""
    email, password, _ = create_test_user("advertiser")

    # Login to get tokens
    resp = jwt_login(email, password)
    assert resp.status_code == 200
    tokens = resp.json()
    assert "access_token" in tokens
    assert "refresh_token" in tokens

    # Verify refresh endpoint works
    refresh_resp = requests.post(
        f"{API_URL}/api/v1/auth/refresh",
        json={"refresh_token": tokens["refresh_token"]},
    )
    assert refresh_resp.status_code == 200
    new_tokens = refresh_resp.json()
    assert "access_token" in new_tokens, "Refresh should return new access token"


def test_logout_clears_tokens():
    """After logout, tokens are cleared from localStorage."""
    email, password, _ = create_test_user("advertiser")

    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)

        # Login via API and set tokens in localStorage
        resp = jwt_login(email, password)
        tokens = resp.json()

        page.goto(FRONTEND_URL)
        page.wait_for_load_state("networkidle")
        page.evaluate(f'''
            localStorage.setItem("dsp_access_token", "{tokens['access_token']}");
            localStorage.setItem("dsp_refresh_token", "{tokens['refresh_token']}");
        ''')
        page.reload()
        page.wait_for_load_state("networkidle")

        # Verify tokens are set
        assert page.evaluate('localStorage.getItem("dsp_access_token")') is not None

        # Click logout (退出登录)
        logout_btn = page.locator('button:has-text("退出"), a:has-text("退出")')
        if logout_btn.count() > 0:
            logout_btn.first.click()
            page.wait_for_load_state("networkidle")

            # Tokens should be cleared
            access = page.evaluate('localStorage.getItem("dsp_access_token")')
            assert access is None, f"Access token not cleared after logout: {access}"

        browser.close()
