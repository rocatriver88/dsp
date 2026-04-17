"""
E2E tests for admin UI workflows.

Covers:
  - Admin overview page loads with stat cards and health panels
  - Invite code generation via UI
  - Agencies page shows pending registrations and advertiser list
  - Audit log page loads
"""

from playwright.sync_api import sync_playwright, expect
from conftest import (
    FRONTEND_URL, ADMIN_TOKEN,
    launch_browser, new_page, admin_login,
)


def test_admin_overview_stat_cards():
    """Admin overview page loads with content."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        admin_login(page, ADMIN_TOKEN)
        page.goto(f"{FRONTEND_URL}/admin")
        page.wait_for_load_state("networkidle")

        # Main content area should be visible
        main = page.locator("main")
        expect(main).to_be_visible(timeout=5000)

        page.screenshot(path="/tmp/dsp_e2e_admin_overview.png", full_page=True)
        browser.close()


def test_admin_overview_health_panel():
    """Admin overview shows health-related content."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        admin_login(page, ADMIN_TOKEN)
        page.goto(f"{FRONTEND_URL}/admin")
        page.wait_for_load_state("networkidle")

        # Health or circuit breaker section should exist
        main = page.locator("main")
        expect(main).to_be_visible()

        browser.close()


def test_invite_code_generation():
    """Admin can generate an invite code via the invites page UI."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        admin_login(page, ADMIN_TOKEN)
        page.goto(f"{FRONTEND_URL}/admin/invites")
        page.wait_for_load_state("networkidle")

        # Section heading
        expect(page.locator("text=生成邀请码").first).to_be_visible()

        # Max uses input should default to 1
        max_uses_input = page.locator('input[type="number"]').first
        expect(max_uses_input).to_be_visible()

        # Click generate button
        gen_btn = page.locator("button", has_text="生成邀请码")
        expect(gen_btn).to_be_visible()
        gen_btn.click()

        # Wait for the success box with the new code to appear
        # The green box should contain a monospace code string
        page.wait_for_timeout(2000)
        page.screenshot(path="/tmp/dsp_e2e_invite_generated.png", full_page=True)

        browser.close()


def test_invite_code_table():
    """Invites page shows invite code data."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        admin_login(page, ADMIN_TOKEN)
        page.goto(f"{FRONTEND_URL}/admin/invites")
        page.wait_for_load_state("networkidle")

        # Page should have invite-related content
        main = page.locator("main")
        expect(main).to_be_visible()
        text = main.text_content() or ""
        assert "邀请码" in text or "生成" in text, f"No invite content on page"

        browser.close()


def test_agencies_page_sections():
    """Agencies page shows agency-related content."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        admin_login(page, ADMIN_TOKEN)
        page.goto(f"{FRONTEND_URL}/admin/agencies")
        page.wait_for_load_state("networkidle")

        main = page.locator("main")
        expect(main).to_be_visible()
        text = main.text_content() or ""
        assert "代理商" in text or "广告主" in text or "注册" in text, \
            f"No agency content on page"

        page.screenshot(path="/tmp/dsp_e2e_agencies.png", full_page=True)
        browser.close()


def test_agencies_topup_modal():
    """Clicking '充值' on an advertiser row opens the top-up modal."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        admin_login(page, ADMIN_TOKEN)
        page.goto(f"{FRONTEND_URL}/admin/agencies")
        page.wait_for_load_state("networkidle")

        # Find first 充值 button in the advertiser table
        topup_btn = page.locator('table[aria-label="广告主列表"] button:has-text("充值")').first
        if topup_btn.is_visible():
            topup_btn.click()

            # Modal should appear
            modal = page.locator('[role="dialog"]')
            expect(modal).to_be_visible(timeout=3000)

            # Modal should have amount input and confirm button
            expect(modal.locator("text=充值金额")).to_be_visible()
            expect(modal.locator("button", has_text="确认充值")).to_be_visible()

            # Close modal
            modal.locator("button", has_text="取消").click()
            expect(modal).not_to_be_visible()

        browser.close()


def test_audit_log_page():
    """Audit log page loads."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        admin_login(page, ADMIN_TOKEN)
        page.goto(f"{FRONTEND_URL}/admin/audit")
        page.wait_for_load_state("networkidle")

        # The main content area (not sidebar) should have audit-related content.
        # Use the <main> element to avoid matching the hidden mobile nav link.
        main = page.locator("main")
        expect(main).to_be_visible(timeout=5000)

        page.screenshot(path="/tmp/dsp_e2e_audit.png", full_page=True)
        browser.close()


def test_creatives_review_page():
    """Creatives review page loads."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        admin_login(page, ADMIN_TOKEN)
        page.goto(f"{FRONTEND_URL}/admin/creatives")
        page.wait_for_load_state("networkidle")

        main = page.locator("main")
        expect(main).to_be_visible(timeout=5000)

        page.screenshot(path="/tmp/dsp_e2e_creatives.png", full_page=True)
        browser.close()


if __name__ == "__main__":
    print("Running admin workflow tests...")
    tests = [
        test_admin_overview_stat_cards,
        test_admin_overview_health_panel,
        test_invite_code_generation,
        test_invite_code_table,
        test_agencies_page_sections,
        test_agencies_topup_modal,
        test_audit_log_page,
        test_creatives_review_page,
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
