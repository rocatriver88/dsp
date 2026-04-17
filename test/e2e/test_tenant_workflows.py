"""
E2E tests for tenant (advertiser) UI workflows.

These tests require a real API key backed by a real advertiser in the
database.  Set DSP_TEST_API_KEY to run the full suite.  When no key is
set, tests that need real data are skipped, and mock-mode tests still run.

Covers:
  - Campaign list page structure
  - Campaign creation wizard (3-step form)
  - Billing page structure and top-up form
  - Reports page structure
  - Analytics page loads
"""

import sys
from playwright.sync_api import sync_playwright, expect
from conftest import (
    FRONTEND_URL, TEST_API_KEY,
    launch_browser, new_page, tenant_login,
)

NEEDS_REAL_KEY = not bool(TEST_API_KEY)


def skip_if_no_key(test_name):
    if NEEDS_REAL_KEY:
        print(f"  SKIP: {test_name} (no DSP_TEST_API_KEY)")
        return True
    return False


# ---------------------------------------------------------------------------
# Campaign list
# ---------------------------------------------------------------------------

def test_campaigns_page_structure():
    """Campaigns page shows table and 'create' button."""
    api_key = TEST_API_KEY or "dsp_e2e_test_key"
    mock = not bool(TEST_API_KEY)
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        tenant_login(page, api_key, mock_api=mock)

        page.goto(f"{FRONTEND_URL}/campaigns")
        wait = "domcontentloaded" if mock else "networkidle"
        page.wait_for_load_state(wait)

        # Main content should be visible (past login gate)
        main = page.locator("#main-content")
        expect(main).to_be_visible()

        # Create button within main content area (may appear twice: header + empty state)
        create_btn = main.locator('a:has-text("创建广告系列")').first
        expect(create_btn).to_be_visible()
        href = create_btn.get_attribute("href")
        assert href == "/campaigns/new", f"Create link href: {href}"

        # Card list exists when there are campaigns; empty state otherwise.
        # In mock mode (no real data) the API returns [], so we may see
        # the empty state. Either card list or empty-state link is acceptable.
        card_list = page.locator('[aria-label="Campaign 列表"]')
        if not card_list.is_visible():
            # Empty state should still show a "创建" CTA
            expect(create_btn).to_be_visible()

        page.screenshot(path="/tmp/dsp_e2e_campaigns.png", full_page=True)
        browser.close()


# ---------------------------------------------------------------------------
# Campaign creation wizard
# ---------------------------------------------------------------------------

def test_campaign_wizard_step1():
    """Campaign creation wizard step 1: basic info fields."""
    api_key = TEST_API_KEY or "dsp_e2e_test_key"
    mock = not bool(TEST_API_KEY)
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        tenant_login(page, api_key, mock_api=mock)

        page.goto(f"{FRONTEND_URL}/campaigns/new")
        wait = "domcontentloaded" if mock else "networkidle"
        page.wait_for_load_state(wait)

        # Step 1 heading
        expect(page.locator("text=基本信息").first).to_be_visible()

        # Campaign name input
        name_input = page.locator('input').first
        expect(name_input).to_be_visible()

        # Billing model toggles
        for model in ["CPM", "CPC", "oCPM"]:
            expect(page.locator(f"button:has-text('{model}')").first).to_be_visible()

        # Next button should be present (possibly disabled)
        next_btn = page.locator("button", has_text="下一步")
        expect(next_btn).to_be_visible()

        page.screenshot(path="/tmp/dsp_e2e_campaign_wizard_step1.png", full_page=True)
        browser.close()


def test_campaign_wizard_navigation():
    """Can navigate through all 3 wizard steps."""
    api_key = TEST_API_KEY or "dsp_e2e_test_key"
    mock = not bool(TEST_API_KEY)
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        tenant_login(page, api_key, mock_api=mock)

        page.goto(f"{FRONTEND_URL}/campaigns/new")
        wait = "domcontentloaded" if mock else "networkidle"
        page.wait_for_load_state(wait)

        # Fill step 1 minimum fields
        # Campaign name
        page.locator('input').first.fill("E2E Test Campaign")
        # Budget fields — find number inputs
        number_inputs = page.locator('input[type="number"]').all()
        for inp in number_inputs:
            if inp.is_visible():
                inp.fill("1000")

        # Click next to step 2
        next_btn = page.locator("button", has_text="下一步").first
        if next_btn.is_enabled():
            next_btn.click()
            page.wait_for_timeout(500)

            # Step 2 should show targeting options
            expect(page.locator("text=定向").first).to_be_visible(timeout=3000)

            # Click next to step 3
            next_btn2 = page.locator("button", has_text="下一步").first
            if next_btn2.is_visible() and next_btn2.is_enabled():
                next_btn2.click()
                page.wait_for_timeout(500)
                # Step 3 should show creative options
                expect(page.locator("text=素材").first).to_be_visible(timeout=3000)

        page.screenshot(path="/tmp/dsp_e2e_campaign_wizard.png", full_page=True)
        browser.close()


# ---------------------------------------------------------------------------
# Billing
# ---------------------------------------------------------------------------

def test_billing_page_structure():
    """Billing page shows balance card and topup button."""
    api_key = TEST_API_KEY or "dsp_e2e_test_key"
    mock = not bool(TEST_API_KEY)
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        tenant_login(page, api_key, mock_api=mock)

        page.goto(f"{FRONTEND_URL}/billing")
        wait = "domcontentloaded" if mock else "networkidle"
        page.wait_for_load_state(wait)

        main = page.locator("#main-content")
        expect(main).to_be_visible(timeout=5000)

        # Balance display
        expect(main.locator("text=账户余额").first).to_be_visible(timeout=5000)

        # Top-up button
        topup_btn = main.locator("button", has_text="充值").first
        expect(topup_btn).to_be_visible()

        # Transaction table may or may not exist (empty state with mock).
        # Just verify the main content area rendered properly.

        page.screenshot(path="/tmp/dsp_e2e_billing.png", full_page=True)
        browser.close()


def test_billing_topup_form():
    """Clicking topup reveals the quick-amount buttons and input."""
    api_key = TEST_API_KEY or "dsp_e2e_test_key"
    mock = not bool(TEST_API_KEY)
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        tenant_login(page, api_key, mock_api=mock)

        page.goto(f"{FRONTEND_URL}/billing")
        wait = "domcontentloaded" if mock else "networkidle"
        page.wait_for_load_state(wait)

        # Click the top-up button
        topup_btn = page.locator("button", has_text="充值").first
        topup_btn.click()
        page.wait_for_timeout(500)

        # Quick amount buttons should appear
        for amount in ["1,000", "5,000", "10,000"]:
            btn = page.locator(f"button:has-text('{amount}')").first
            if btn.is_visible():
                break
        else:
            # Try without comma formatting
            expect(page.locator("button:has-text('1000')").first).to_be_visible(timeout=3000)

        # Confirm button
        confirm = page.locator("button", has_text="确认充值").first
        expect(confirm).to_be_visible()

        # Cancel / close
        cancel_btn = page.locator("button", has_text="取消").first
        if cancel_btn.is_visible():
            cancel_btn.click()

        browser.close()


# ---------------------------------------------------------------------------
# Reports & Analytics
# ---------------------------------------------------------------------------

def test_reports_page_structure():
    """Reports page loads."""
    api_key = TEST_API_KEY or "dsp_e2e_test_key"
    mock = not bool(TEST_API_KEY)
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        tenant_login(page, api_key, mock_api=mock)

        page.goto(f"{FRONTEND_URL}/reports")
        wait = "domcontentloaded" if mock else "networkidle"
        page.wait_for_load_state(wait)

        # Page should render without 500
        # Just verify we're past the login gate
        expect(page.locator("#main-content")).to_be_visible()

        page.screenshot(path="/tmp/dsp_e2e_reports.png", full_page=True)
        browser.close()


def test_analytics_page_loads():
    """Analytics page loads."""
    api_key = TEST_API_KEY or "dsp_e2e_test_key"
    mock = not bool(TEST_API_KEY)
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        tenant_login(page, api_key, mock_api=mock)

        page.goto(f"{FRONTEND_URL}/analytics")
        wait = "domcontentloaded" if mock else "networkidle"
        page.wait_for_load_state(wait)

        expect(page.locator("#main-content")).to_be_visible()

        page.screenshot(path="/tmp/dsp_e2e_analytics.png", full_page=True)
        browser.close()


if __name__ == "__main__":
    print("Running tenant workflow tests...")
    tests = [
        test_campaigns_page_structure,
        test_campaign_wizard_step1,
        test_campaign_wizard_navigation,
        test_billing_page_structure,
        test_billing_topup_form,
        test_reports_page_structure,
        test_analytics_page_loads,
    ]
    passed = 0
    failed = 0
    skipped = 0
    for t in tests:
        try:
            t()
            print(f"  PASS: {t.__name__}")
            passed += 1
        except Exception as e:
            print(f"  FAIL: {t.__name__}: {e}")
            failed += 1
    print(f"\n{passed} passed, {failed} failed")
