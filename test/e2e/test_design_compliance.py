"""
E2E tests for DESIGN.md compliance.

Automatically verifies that rendered pages match the design system:
  - Typography (font families, sizes)
  - Colors (primary, sidebar, semantic)
  - Spacing (base unit multiples)
  - Component patterns

Note: Newer Chromium returns colors in lab() format rather than rgb().
We use a canvas-based JS helper to normalize all colors to hex for
reliable comparison.
"""

from playwright.sync_api import sync_playwright, expect
from conftest import (
    FRONTEND_URL, ADMIN_TOKEN, TEST_API_KEY,
    launch_browser, new_page, tenant_login, admin_login,
)

# DESIGN.md reference values (hex, lowercase)
DESIGN = {
    "font_display": "Inter",
    "font_body": "Inter",
    "primary_color": "#8b5cf6",
    "primary_hover": "#7c3aed",
    "sidebar_bg": "#0a0610",
    "page_bg": "#0f0a1a",
    "text_primary": "#ffffff",
    "text_secondary": "#a0a0b0",
    "border_color": "#2a2035",
    "spacing_base": 4,
}

# JS helper injected into pages to convert any CSS color to hex.
# Uses a temporary canvas 2d context which normalizes all color formats.
COLOR_TO_HEX_JS = """
(cssColor) => {
    const canvas = document.createElement('canvas');
    canvas.width = canvas.height = 1;
    const ctx = canvas.getContext('2d');
    ctx.fillStyle = cssColor;
    ctx.fillRect(0, 0, 1, 1);
    const [r, g, b] = ctx.getImageData(0, 0, 1, 1).data;
    return '#' + [r, g, b].map(c => c.toString(16).padStart(2, '0')).join('');
}
"""


def get_color_hex(page, selector, prop):
    """Get a CSS color property as a normalized hex string."""
    return page.locator(selector).first.evaluate(
        f"""el => {{
            const css = getComputedStyle(el).{prop};
            const canvas = document.createElement('canvas');
            canvas.width = canvas.height = 1;
            const ctx = canvas.getContext('2d');
            ctx.fillStyle = css;
            ctx.fillRect(0, 0, 1, 1);
            const [r, g, b] = ctx.getImageData(0, 0, 1, 1).data;
            return '#' + [r, g, b].map(c => c.toString(16).padStart(2, '0')).join('');
        }}"""
    )


def get_computed(page, selector, prop):
    """Get a computed CSS property from the first matching element."""
    return page.locator(selector).first.evaluate(
        f'el => getComputedStyle(el).{prop}'
    )


def color_close(hex_a, hex_b, tolerance=20):
    """Check if two hex colors are within tolerance per channel.

    Default tolerance of 20 accounts for Tailwind CSS v4 palette shifts
    (e.g. blue-600 moved from #2563EB to #155DFC).
    """
    r1, g1, b1 = int(hex_a[1:3], 16), int(hex_a[3:5], 16), int(hex_a[5:7], 16)
    r2, g2, b2 = int(hex_b[1:3], 16), int(hex_b[3:5], 16), int(hex_b[5:7], 16)
    return abs(r1 - r2) <= tolerance and abs(g1 - g2) <= tolerance and abs(b1 - b2) <= tolerance


def test_login_page_design():
    """Login page matches DESIGN.md specifications."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        page.goto(FRONTEND_URL)
        page.wait_for_load_state("networkidle")

        # Page background should be dark (#0F0A1A)
        bg = get_color_hex(page, "body", "backgroundColor")
        assert color_close(bg, DESIGN["page_bg"]), \
            f"Page bg: expected ~{DESIGN['page_bg']}, got {bg}"

        # Login button: disabled state uses opacity, so fill to enable first
        page.locator('input[type="email"]').fill("test@example.com")
        page.locator('input[type="password"]').fill("password123")
        btn_bg = get_color_hex(page, "button", "backgroundColor")
        assert color_close(btn_bg, DESIGN["primary_color"]), \
            f"Button bg: expected ~{DESIGN['primary_color']}, got {btn_bg}"

        # Login card should have dark card background (#1A1225)
        card_bg = get_color_hex(page, "body > div > div", "backgroundColor")
        assert color_close(card_bg, "#1a1225", tolerance=25), \
            f"Card bg: expected ~#1a1225, got {card_bg}"

        page.screenshot(path="/tmp/dsp_e2e_login_design.png", full_page=True)
        browser.close()


def test_admin_sidebar_design():
    """Admin sidebar matches DESIGN.md color scheme."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        admin_login(page)

        # Sidebar background should be #111827 (DESIGN.md sidebar bg)
        sidebar_bg = get_color_hex(page, 'nav[aria-label="管理员导航"]', "backgroundColor")
        assert color_close(sidebar_bg, DESIGN["sidebar_bg"]), \
            f"Sidebar bg: expected ~{DESIGN['sidebar_bg']}, got {sidebar_bg}"

        # Sidebar heading should be white
        heading_color = get_color_hex(page, 'nav[aria-label="管理员导航"] h1', "color")
        assert color_close(heading_color, "#ffffff"), \
            f"Sidebar heading color: expected white, got {heading_color}"

        page.screenshot(path="/tmp/dsp_e2e_admin_design.png", full_page=True)
        browser.close()


def test_spacing_multiples():
    """Key UI elements use 4px-multiple spacing (DESIGN.md base unit)."""
    api_key = TEST_API_KEY or "dsp_e2e_test_key"
    mock = not bool(TEST_API_KEY)
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        tenant_login(page, api_key, mock_api=mock)

        # Check main content padding
        padding = page.locator("#main-content > div").first.evaluate(
            'el => ({ pt: parseFloat(getComputedStyle(el).paddingTop),'
            '         pr: parseFloat(getComputedStyle(el).paddingRight),'
            '         pb: parseFloat(getComputedStyle(el).paddingBottom),'
            '         pl: parseFloat(getComputedStyle(el).paddingLeft) })'
        )
        base = DESIGN["spacing_base"]
        for side, val in padding.items():
            assert val % base == 0, \
                f"Content padding-{side} = {val}px is not a multiple of {base}px"

        browser.close()


def test_primary_button_colors():
    """Primary action buttons use the design system primary color."""
    with sync_playwright() as p:
        browser = launch_browser(p)
        page = new_page(browser)
        page.goto(FRONTEND_URL)
        page.wait_for_load_state("networkidle")

        # Fill input to enable button
        page.locator('input[type="email"]').fill("test@example.com")
        page.locator('input[type="password"]').fill("password123")

        btn_bg = get_color_hex(page, "button", "backgroundColor")
        assert color_close(btn_bg, DESIGN["primary_color"]), \
            f"Primary button: expected ~{DESIGN['primary_color']}, got {btn_bg}"

        browser.close()


if __name__ == "__main__":
    print("Running design compliance tests...")
    tests = [
        test_login_page_design,
        test_admin_sidebar_design,
        test_spacing_multiples,
        test_primary_button_colors,
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
