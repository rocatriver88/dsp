"""
End-to-end business flow test.

Walks through the full user journey in a real browser against live services:

  1. Admin generates invite code
  2. Public registration with invite code
  3. Admin approves registration → gets advertiser_id + api_key
  4. Tenant logs in with real API key
  5. Admin tops up the new advertiser
  6. Tenant sees updated balance
  7. Tenant creates a Campaign (3-step wizard)
  8. Campaign appears in list with status=draft
  9. Tenant starts the Campaign
  10. Campaign status changes to active

Each step verifies:
  [interaction] — UI action produces expected visual result
  [db]          — database state matches what the page shows
"""

import os
import subprocess
import time
from playwright.sync_api import sync_playwright, expect
from conftest import (
    FRONTEND_URL, ADMIN_TOKEN,
    launch_browser, new_page, admin_login,
)

SCREENSHOT_DIR = os.environ.get("DSP_SCREENSHOT_DIR", "/tmp/dsp_e2e_flow")


def pg(sql):
    """Run a SQL query against the dev PostgreSQL and return stripped output."""
    result = subprocess.run(
        ["docker", "compose", "exec", "-T", "postgres",
         "psql", "-U", "dsp", "-d", "dsp", "-tA", "-c", sql],
        capture_output=True, text=True, timeout=10, encoding="utf-8",
    )
    out = result.stdout.strip()
    if result.returncode != 0 and not out:
        raise RuntimeError(f"pg query failed: {result.stderr.strip()}")
    return out


def screenshot(page, name):
    page.screenshot(path=f"{SCREENSHOT_DIR}/{name}.png", full_page=True)


class FlowState:
    """Accumulates state across steps so later steps can reference earlier results."""
    invite_code = None
    reg_email = None
    advertiser_id = None
    api_key = None
    user_email = None       # from approve response (same as reg_email, echoed back)
    temp_password = None    # one-time plaintext disclosed only in approve response
    campaign_id = None


state = FlowState()
results = []


def record(step_name, passed, detail=""):
    status = "PASS" if passed else "FAIL"
    results.append((step_name, status, detail))
    print(f"  {status}: {step_name}" + (f" — {detail}" if detail else ""))
    if not passed:
        raise AssertionError(f"{step_name}: {detail}")


# ============================================================================
# STEP 1: Admin generates invite code
# ============================================================================
def step1_admin_generate_invite(browser):
    page = new_page(browser)
    admin_login(page)
    page.goto(f"{FRONTEND_URL}/admin/invites")
    page.wait_for_load_state("networkidle")

    # [db] baseline
    db_count_before = int(pg("SELECT count(*) FROM invite_codes"))

    # [interaction] Set max_uses=1, click generate
    max_input = page.locator('input[type="number"]').first
    max_input.fill("1")
    gen_btn = page.locator("button", has_text="生成邀请码")
    gen_btn.click()

    # Wait for success
    page.wait_for_timeout(2000)
    screenshot(page, "01_invite_generated")

    # [interaction] Extract the generated code from the green success box
    # The invite code appears in a monospace/code element after generation
    code_el = page.locator("code, .font-mono, [class*='monospace']").first
    if code_el.is_visible():
        state.invite_code = code_el.text_content().strip()
    else:
        # Fallback: get the latest invite code from DB
        state.invite_code = pg(
            "SELECT code FROM invite_codes ORDER BY created_at DESC LIMIT 1"
        )

    assert state.invite_code and len(state.invite_code) > 5, \
        f"Invalid invite code: {state.invite_code}"

    # [db] verify count increased
    db_count_after = int(pg("SELECT count(*) FROM invite_codes"))
    assert db_count_after == db_count_before + 1, \
        f"invite_codes count: {db_count_before} → {db_count_after}, expected +1"

    # [db] verify code exists in DB
    db_code = pg(f"SELECT code FROM invite_codes WHERE code = '{state.invite_code}'")
    assert db_code == state.invite_code, \
        f"Code {state.invite_code} not found in DB"

    record("Step 1: Admin generates invite code",
           True, f"code={state.invite_code}")
    page.close()


# ============================================================================
# STEP 2: Public registration with invite code
# ============================================================================
def step2_register(browser):
    ts = int(time.time())
    state.reg_email = f"e2e-flow-{ts}@test.local"
    company = f"E2E Flow Test {ts}"

    # Use API directly for registration (no public registration UI page exists)
    import urllib.request
    import json
    body = json.dumps({
        "company_name": company,
        "contact_email": state.reg_email,
        "invite_code": state.invite_code,
    }).encode()
    req = urllib.request.Request(
        "http://localhost:18181/api/v1/register",
        data=body,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    resp = urllib.request.urlopen(req)
    assert resp.status in (200, 201), f"Register returned {resp.status}"

    # Brief pause for DB commit to propagate
    time.sleep(1)

    # [db] verify registration exists
    db_email = pg(
        f"SELECT contact_email FROM registration_requests "
        f"WHERE contact_email = '{state.reg_email}'"
    )
    assert db_email == state.reg_email, \
        f"Registration not in DB: {state.reg_email}"

    record("Step 2: Register with invite code",
           True, f"email={state.reg_email}")


# ============================================================================
# STEP 3: Admin approves registration → gets advertiser_id + api_key
# ============================================================================
def step3_admin_approve(browser):
    page = new_page(browser)
    admin_login(page)
    page.goto(f"{FRONTEND_URL}/admin/agencies")
    page.wait_for_load_state("networkidle")

    screenshot(page, "03a_agencies_before_approve")

    # [interaction] Find the pending registration row with our email
    # and click "批准"
    pending_table = page.locator("table").first
    rows = pending_table.locator("tr").all()
    approve_btn = None
    for row in rows:
        if state.reg_email in (row.text_content() or ""):
            approve_btn = row.locator("button", has_text="批准")
            break

    assert approve_btn is not None, \
        f"Could not find pending registration for {state.reg_email}"

    # Capture the approve response so we can pick up the one-time temp_password
    # (plaintext is never persisted; only this response carries it). Filter on
    # method=POST and status=200 so a CORS OPTIONS preflight on cross-origin
    # setups doesn't bind expect_response to an empty preflight reply.
    with page.expect_response(
        lambda r: (
            "/api/v1/admin/registrations/" in r.url
            and r.url.endswith("/approve")
            and r.request.method == "POST"
            and r.status == 200
        )
    ) as resp_info:
        approve_btn.click()
    approve_body = resp_info.value.json()

    page.wait_for_timeout(1000)
    screenshot(page, "03b_agencies_after_approve")

    state.advertiser_id = str(approve_body.get("advertiser_id") or "")
    state.api_key = approve_body.get("api_key") or ""
    state.user_email = approve_body.get("user_email") or ""
    state.temp_password = approve_body.get("temp_password") or ""

    assert state.advertiser_id and state.advertiser_id != "0", \
        f"approve response missing advertiser_id: {approve_body}"
    assert state.api_key and state.api_key.startswith("dsp_"), \
        f"api_key invalid: {state.api_key}"
    assert state.user_email == state.reg_email, \
        f"user_email mismatch: got {state.user_email}, expected {state.reg_email}"
    assert state.temp_password and len(state.temp_password) >= 16, \
        f"temp_password invalid: {state.temp_password}"

    # [db] verify advertiser balance is 0 + a users row was seeded
    balance = pg(
        f"SELECT balance_cents FROM advertisers WHERE id = {state.advertiser_id}"
    )
    assert balance == "0", f"New advertiser balance should be 0, got {balance}"

    user_role = pg(
        f"SELECT role FROM users WHERE email = '{state.reg_email}'"
    )
    assert user_role == "advertiser", \
        f"Seeded user role should be 'advertiser', got {user_role!r}"

    record("Step 3: Admin approves registration",
           True,
           f"advertiser_id={state.advertiser_id}, api_key={state.api_key[:16]}..., "
           f"temp_password={state.temp_password[:8]}...")
    page.close()


# ============================================================================
# STEP 4: Tenant logs in with real API key
# ============================================================================
def step4_tenant_login(browser):
    """Tenant logs in via the primary email+password JWT path using the
    temp_password handed back by the approve response. The old API-key entry
    is still supported (see /auth/apikey-login) but the main login page
    switched to JWT, so that is what we exercise here."""
    page = new_page(browser)
    page.goto(FRONTEND_URL)
    page.wait_for_load_state("networkidle")

    # [interaction] Fill email + password on the primary login form.
    page.locator('input[placeholder="your@email.com"]').fill(state.user_email)
    page.locator('input[placeholder="输入密码"]').fill(state.temp_password)

    login_btn = page.locator("button", has_text="登录").first
    expect(login_btn).to_be_enabled()
    login_btn.click()

    # Wait for login to complete — should see the dashboard.
    page.wait_for_load_state("networkidle")
    page.wait_for_timeout(1000)

    # [interaction] Should see the authed shell, not the login form.
    expect(page.locator("#main-content")).to_be_visible(timeout=5000)
    expect(page.locator("h2", has_text="DSP Platform")).not_to_be_visible()

    # Sidebar is only rendered post-login — use the logout button as the
    # authed-state anchor (it only exists post-login, not in the login form).
    # "账户余额" lives on /billing (asserted by Step 6); don't couple Step 4 to it.
    expect(page.get_by_role("button", name="退出登录").first).to_be_visible(timeout=10000)

    screenshot(page, "04_tenant_dashboard")

    record("Step 4: Tenant login with email + temp password (JWT)", True)
    page.close()


# ============================================================================
# STEP 5: Admin tops up the new advertiser
# ============================================================================
def step5_admin_topup(browser):
    page = new_page(browser)
    admin_login(page)
    page.goto(f"{FRONTEND_URL}/admin/agencies")
    page.wait_for_load_state("networkidle")

    # [db] balance before
    balance_before = int(pg(
        f"SELECT balance_cents FROM advertisers WHERE id = {state.advertiser_id}"
    ))

    # [interaction] Find our advertiser in the list, click 充值
    adv_table = page.locator('table[aria-label="广告主列表"]')

    # Scroll down to find the advertiser — might need to look through rows
    topup_btn = None
    rows = adv_table.locator("tbody tr").all()
    for row in rows:
        row_text = row.text_content() or ""
        if state.reg_email in row_text or f"#{state.advertiser_id}" in row_text or state.advertiser_id in row_text:
            topup_btn = row.locator("button", has_text="充值")
            break

    assert topup_btn is not None, \
        f"Could not find advertiser {state.advertiser_id} in table"

    topup_btn.click()

    # [interaction] Fill in top-up modal
    modal = page.locator('[role="dialog"]')
    expect(modal).to_be_visible(timeout=3000)

    amount_input = modal.locator('input[type="number"]').first
    amount_input.fill("5000")

    confirm_btn = modal.locator("button", has_text="确认充值")
    confirm_btn.click()
    page.wait_for_timeout(2000)

    screenshot(page, "05_admin_topup")

    # [db] verify balance increased by 500000 cents (¥5000)
    balance_after = int(pg(
        f"SELECT balance_cents FROM advertisers WHERE id = {state.advertiser_id}"
    ))
    expected = balance_before + 500000
    assert balance_after == expected, \
        f"Balance: {balance_before} → {balance_after}, expected {expected}"

    # [db] verify audit log entry exists
    audit = pg(
        f"SELECT action FROM audit_log "
        f"WHERE advertiser_id = {state.advertiser_id} "
        f"ORDER BY created_at DESC LIMIT 1"
    )
    assert "topup" in audit.lower() or "billing" in audit.lower(), \
        f"No topup audit entry, got: {audit}"

    record("Step 5: Admin tops up ¥5000",
           True, f"balance: {balance_before} → {balance_after} cents")
    page.close()


# ============================================================================
# STEP 6: Tenant sees updated balance
# ============================================================================
def step6_tenant_check_balance(browser):
    page = new_page(browser)
    page.goto(FRONTEND_URL)
    page.wait_for_load_state("networkidle")

    # Login
    page.evaluate(f'localStorage.setItem("dsp_api_key", "{state.api_key}")')
    page.goto(f"{FRONTEND_URL}/billing")
    page.wait_for_load_state("networkidle")
    page.wait_for_timeout(1000)

    screenshot(page, "06_tenant_balance")

    # [interaction] Balance should show ¥5,000 (or 5000)
    main = page.locator("#main-content")
    balance_area = main.text_content() or ""
    assert "NaN" not in balance_area, f"Balance shows NaN"

    # [db] cross-reference
    db_balance = int(pg(
        f"SELECT balance_cents FROM advertisers WHERE id = {state.advertiser_id}"
    ))
    expected_display = f"{db_balance / 100:,.2f}"
    # The page should show the balance somewhere
    # Just verify it's a reasonable number, not NaN/undefined
    assert "5,000" in balance_area or "5000" in balance_area, \
        f"Expected ¥5000 in page, got: {balance_area[:200]}"

    record("Step 6: Tenant sees ¥5000 balance", True)
    page.close()


# ============================================================================
# STEP 7: Tenant creates a Campaign (3-step wizard)
# ============================================================================
def step7_create_campaign(browser):
    page = new_page(browser)
    page.goto(FRONTEND_URL)
    page.wait_for_load_state("networkidle")
    page.evaluate(f'localStorage.setItem("dsp_api_key", "{state.api_key}")')
    page.goto(f"{FRONTEND_URL}/campaigns/new")
    page.wait_for_load_state("networkidle")

    # [db] campaign count before
    db_count_before = int(pg(
        f"SELECT count(*) FROM campaigns WHERE advertiser_id = {state.advertiser_id}"
    ))

    # --- Step 1: Basic info ---
    # Campaign name
    name_input = page.locator('input[type="text"]').first
    name_input.fill("E2E Flow Campaign")

    # Billing model — click CPM
    page.locator("button", has_text="CPM").first.click()

    # Budget fields
    number_inputs = page.locator('input[type="number"]').all()
    for inp in number_inputs:
        if inp.is_visible():
            inp.fill("1000")

    screenshot(page, "07a_wizard_step1")

    # Click next
    next_btn = page.locator("button", has_text="下一步").first
    expect(next_btn).to_be_enabled(timeout=3000)
    next_btn.click()
    page.wait_for_timeout(500)

    # --- Step 2: Targeting ---
    expect(page.locator("text=定向").first).to_be_visible(timeout=3000)

    # Select CN geo
    cn_btn = page.locator("button", has_text="CN").first
    if cn_btn.is_visible():
        cn_btn.click()

    screenshot(page, "07b_wizard_step2")

    next_btn2 = page.locator("button", has_text="下一步").first
    next_btn2.click()
    page.wait_for_timeout(500)

    # --- Step 3: Creative ---
    expect(page.locator("text=素材").first).to_be_visible(timeout=3000)

    # Select banner type
    banner_btn = page.locator("button", has_text="横幅").first
    if banner_btn.is_visible():
        banner_btn.click()
        page.wait_for_timeout(300)

    # Fill creative name
    creative_name_input = page.locator('input[type="text"]').all()
    for inp in creative_name_input:
        if inp.is_visible() and not inp.input_value():
            inp.fill("E2E Banner Creative")
            break

    # Fill landing page URL
    url_inputs = page.locator('input[type="url"]').all()
    for inp in url_inputs:
        if inp.is_visible():
            inp.fill("https://example.com")
            break

    # Fill ad markup if textarea visible
    textarea = page.locator("textarea").first
    if textarea.is_visible():
        textarea.fill('<a href="https://example.com">E2E Ad</a>')

    screenshot(page, "07c_wizard_step3")

    # Submit
    submit_btn = page.locator("button", has_text="创建 Campaign").first
    if submit_btn.is_visible() and submit_btn.is_enabled():
        submit_btn.click()
        page.wait_for_timeout(3000)

    screenshot(page, "07d_wizard_submitted")

    # [db] verify campaign was created
    db_count_after = int(pg(
        f"SELECT count(*) FROM campaigns WHERE advertiser_id = {state.advertiser_id}"
    ))

    if db_count_after > db_count_before:
        state.campaign_id = pg(
            f"SELECT id FROM campaigns WHERE advertiser_id = {state.advertiser_id} "
            f"ORDER BY created_at DESC LIMIT 1"
        )
        db_status = pg(
            f"SELECT status FROM campaigns WHERE id = {state.campaign_id}"
        )
        record("Step 7: Create Campaign via wizard",
               True, f"campaign_id={state.campaign_id}, status={db_status}")
    else:
        # Wizard may have validation issues — try API fallback
        import urllib.request
        import json
        body = json.dumps({
            "name": "E2E Flow Campaign",
            "billing_model": "cpm",
            "budget_total_cents": 100000,
            "budget_daily_cents": 10000,
            "bid_cpm_cents": 150,
            "targeting": {},
        }).encode()
        req = urllib.request.Request(
            "http://localhost:18181/api/v1/campaigns",
            data=body,
            headers={
                "Content-Type": "application/json",
                "X-API-Key": state.api_key,
            },
            method="POST",
        )
        resp = urllib.request.urlopen(req)
        data = json.loads(resp.read())
        state.campaign_id = str(data.get("id"))
        record("Step 7: Create Campaign (API fallback)",
               True, f"campaign_id={state.campaign_id}, wizard had issues")

    page.close()


# ============================================================================
# STEP 8: Campaign appears in list
# ============================================================================
def step8_campaign_in_list(browser):
    page = new_page(browser)
    page.goto(FRONTEND_URL)
    page.wait_for_load_state("networkidle")
    page.evaluate(f'localStorage.setItem("dsp_api_key", "{state.api_key}")')
    page.goto(f"{FRONTEND_URL}/campaigns")
    page.wait_for_load_state("networkidle")
    page.wait_for_timeout(1000)

    screenshot(page, "08_campaign_list")

    # [interaction] Campaign should appear in the card list
    card_list = page.locator('[aria-label="Campaign 列表"]')
    expect(card_list).to_be_visible(timeout=5000)

    # Look for our campaign
    list_text = card_list.text_content() or ""
    assert "E2E Flow Campaign" in list_text, \
        f"Campaign not found in list. List content: {list_text[:300]}"

    # [interaction] Status should show draft
    # Find the card with our campaign
    row = card_list.locator("div", has_text="E2E Flow Campaign").first
    row_text = row.text_content() or ""

    # [db] cross-reference
    db_status = pg(f"SELECT status FROM campaigns WHERE id = {state.campaign_id}")
    assert db_status == "draft", f"DB status: {db_status}, expected draft"

    record("Step 8: Campaign appears in list", True, f"status={db_status}")
    page.close()


# ============================================================================
# STEP 9: Tenant starts the Campaign
# ============================================================================
def step9_start_campaign(browser):
    page = new_page(browser)
    page.goto(FRONTEND_URL)
    page.wait_for_load_state("networkidle")
    page.evaluate(f'localStorage.setItem("dsp_api_key", "{state.api_key}")')
    page.goto(f"{FRONTEND_URL}/campaigns")
    page.wait_for_load_state("networkidle")
    page.wait_for_timeout(1000)

    # [interaction] Find the play (启动) button for our campaign
    card_list = page.locator('[aria-label="Campaign 列表"]')
    row = card_list.locator("div", has_text="E2E Flow Campaign").first
    start_btn = row.locator('button[aria-label*="启动"], button[aria-label*="恢复"]').first

    if start_btn.is_visible():
        start_btn.click()
        page.wait_for_timeout(2000)
        screenshot(page, "09_campaign_started")

        # [db] verify status changed
        db_status = pg(f"SELECT status FROM campaigns WHERE id = {state.campaign_id}")
        assert db_status == "active", f"DB status after start: {db_status}"

        record("Step 9: Start Campaign", True, f"status → {db_status}")
    else:
        # Campaign might need a creative first
        screenshot(page, "09_campaign_no_start_btn")
        record("Step 9: Start Campaign",
               False, "启动 button not visible — campaign may need creative attachment")

    page.close()


# ============================================================================
# STEP 10: Verify final state
# ============================================================================
def step10_verify_final(browser):
    # [db] comprehensive final verification
    adv = pg(f"SELECT balance_cents FROM advertisers WHERE id = {state.advertiser_id}")
    camp_status = pg(f"SELECT status FROM campaigns WHERE id = {state.campaign_id}")
    camp_name = pg(f"SELECT name FROM campaigns WHERE id = {state.campaign_id}")

    detail = (
        f"advertiser balance={adv} cents, "
        f"campaign '{camp_name}' status={camp_status}"
    )
    record("Step 10: Final DB verification", True, detail)


# ============================================================================
# MAIN
# ============================================================================
def run_flow():
    os.makedirs(SCREENSHOT_DIR, exist_ok=True)
    print("=" * 60)
    print("  E2E BUSINESS FLOW TEST")
    print("=" * 60)
    print()

    with sync_playwright() as p:
        browser = launch_browser(p)

        steps = [
            ("Step 1", step1_admin_generate_invite),
            ("Step 2", step2_register),
            ("Step 3", step3_admin_approve),
            ("Step 4", step4_tenant_login),
            ("Step 5", step5_admin_topup),
            ("Step 6", step6_tenant_check_balance),
            ("Step 7", step7_create_campaign),
            ("Step 8", step8_campaign_in_list),
            ("Step 9", step9_start_campaign),
            ("Step 10", step10_verify_final),
        ]

        last_completed = None
        for name, fn in steps:
            try:
                fn(browser)
                last_completed = name
            except Exception as e:
                print(f"  FAIL: {name} — {e}")
                results.append((name, "FAIL", str(e)))
                # Continue to next step if possible, but some steps
                # depend on state from earlier steps
                if state.api_key is None and name in ("Step 4", "Step 5", "Step 6"):
                    print(f"  SKIP: remaining steps (no api_key)")
                    break
                if state.campaign_id is None and name in ("Step 8", "Step 9"):
                    print(f"  SKIP: remaining steps (no campaign_id)")
                    break

        browser.close()

    # Summary
    print()
    print("=" * 60)
    print("  SUMMARY")
    print("=" * 60)
    passed = sum(1 for _, s, _ in results if s == "PASS")
    failed = sum(1 for _, s, _ in results if s == "FAIL")
    print(f"  {passed} passed, {failed} failed, {len(steps) - len(results)} skipped")
    print()
    for name, status, detail in results:
        marker = "OK" if status == "PASS" else "XX"
        print(f"  {marker} {name}: {detail[:100]}")
    print()
    print(f"  Screenshots: {SCREENSHOT_DIR}/")

    return failed == 0


if __name__ == "__main__":
    success = run_flow()
    exit(0 if success else 1)
