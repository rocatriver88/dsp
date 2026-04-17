"""Quick check of the analytics page with a real API key."""
from playwright.sync_api import sync_playwright
from conftest import FRONTEND_URL, launch_browser, new_page

API_KEY = "dsp_fe554a19bf2c63f0fef7c6d54df02342c02d11ed1b04fa2f94af8b7a2ab100a0"

with sync_playwright() as p:
    browser = launch_browser(p)
    page = new_page(browser)

    # Collect console messages
    console_msgs = []
    page.on("console", lambda msg: console_msgs.append(f"[{msg.type}] {msg.text}"))

    page.goto(FRONTEND_URL)
    page.wait_for_load_state("networkidle")
    page.evaluate(f'localStorage.setItem("dsp_api_key", "{API_KEY}")')
    page.goto(f"{FRONTEND_URL}/analytics")
    page.wait_for_load_state("domcontentloaded")
    page.wait_for_timeout(5000)  # give SSE time to deliver data

    page.screenshot(
        path="C:/Users/Roc/github/dsp/docs/qa/e2e-flow/analytics_realkey.png",
        full_page=True,
    )

    # Report
    main = page.locator("#main-content")
    if main.is_visible():
        text = main.text_content() or ""
        print(f"Main content length: {len(text)} chars")
        print(f"First 500 chars: {text[:500]}")
    else:
        print("NO #main-content visible")

    skeleton_count = page.locator("[class*='animate-pulse']").count()
    print(f"Skeleton/pulse elements: {skeleton_count}")

    print(f"\nConsole messages ({len(console_msgs)}):")
    for msg in console_msgs[:20]:
        print(f"  {msg}")

    browser.close()
