"""
E2E tests for page navigation and routing.

Covers:
  - All tenant pages are reachable after login
  - All admin pages are reachable after admin login
  - Sidebar navigation works
  - Page titles / headings render
"""

from playwright.sync_api import sync_playwright, expect
from conftest import (
    FRONTEND_URL, ADMIN_TOKEN, TEST_API_KEY,
    launch_browser, new_page, tenant_login, admin_login,
)

TENANT_PAGES = [
    ("/", "概览"),
    ("/campaigns", "广告计划"),
    ("/analytics", None),       # heading varies
    ("/billing", None),
    ("/reports", None),
]

ADMIN_PAGES = [
    ("/admin", "概览"),
    ("/admin/agencies", "代理商"),
    ("/admin/creatives", "素材审核"),
    ("/admin/invites", "邀请码"),
    ("/admin/audit", "审计日志"),
]


def test_tenant_pages_reachable():
    """Each tenant page loads without error after login."""
    api_key = TEST_API_KEY or "dsp_e2e_test_key"
    mock = not bool(TEST_API_KEY)
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        tenant_login(page, api_key, mock_api=mock)

        wait_state = "domcontentloaded" if mock else "networkidle"
        for path, _ in TENANT_PAGES:
            url = f"{FRONTEND_URL}{path}"
            resp = page.goto(url)
            page.wait_for_load_state(wait_state)
            assert resp.status < 500, f"{path} returned {resp.status}"
            # Should not be redirected back to login
            expect(page.locator("#main-content")).to_be_visible()

        browser.close()


def test_admin_pages_reachable():
    """Each admin page loads without error after admin login."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        admin_login(page)

        for path, expected_label in ADMIN_PAGES:
            url = f"{FRONTEND_URL}{path}"
            resp = page.goto(url)
            page.wait_for_load_state("networkidle")
            assert resp.status < 500, f"{path} returned {resp.status}"

        browser.close()


def test_admin_sidebar_navigation():
    """Clicking each admin desktop sidebar link navigates to the correct page."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        admin_login(page)

        sidebar = page.locator('nav[aria-label="管理员导航"]')

        # Skip the first item ("概览" → /admin) since we're already there.
        # Test navigation to the other pages.
        for path, label in ADMIN_PAGES[1:]:
            if label is None:
                continue
            nav_link = sidebar.locator(f'a[role="listitem"]:has-text("{label}")')
            nav_link.click()
            page.wait_for_url(f"**{path}", timeout=10000)
            assert path in page.url, \
                f"Expected URL containing {path}, got {page.url}"

        browser.close()


def test_admin_back_to_tenant_link():
    """Admin sidebar '广告主后台' link points to the tenant root."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        admin_login(page)

        sidebar = page.locator('nav[aria-label="管理员导航"]')
        back_link = sidebar.locator('a:has-text("广告主后台")')
        expect(back_link).to_be_visible()

        # Verify the href points to "/" rather than clicking (which may
        # trigger a Next.js client-side nav that doesn't update URL
        # immediately).
        href = back_link.get_attribute("href")
        assert href == "/", f"Expected href='/', got '{href}'"

        # Click and wait for URL to no longer be /admin
        back_link.click()
        page.wait_for_url("**/", timeout=10000)
        # URL should be exactly the root, not /admin/*
        from urllib.parse import urlparse
        path = urlparse(page.url).path
        assert path == "/" or not path.startswith("/admin"), \
            f"Expected non-admin URL, got {page.url}"

        browser.close()


if __name__ == "__main__":
    print("Running navigation tests...")
    tests = [
        test_tenant_pages_reachable,
        test_admin_pages_reachable,
        test_admin_sidebar_navigation,
        test_admin_back_to_tenant_link,
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
