"""Check the flight easter egg in one isolated browser, without a database or model calls."""
import argparse
import json
import mimetypes
from pathlib import Path
import subprocess
from urllib.parse import urlparse

ROOT = Path(__file__).resolve().parents[1]
WEB = ROOT / "internal/server/web"


def verify_flight_easter_egg(page, screenshot=None):
    """Exercise the shared scene over a signed-in workspace without changing its data."""
    page.locator('#nav-views [data-view="Todos"]').focus()
    page.evaluate("document.querySelector('main').style.minHeight='2400px';window.scrollTo(0,500)")
    page.evaluate("window.flightMain=document.querySelector('main');window.flightOpener=document.activeElement;window.flightView=view;window.flightProject=projectId;window.flightScroll=scrollY")
    for code in ("idkfa", "iddqd"):
        page.keyboard.type(code)
        page.wait_for_function("!!landingFlight&&!!document.querySelector('.flight-easter-egg[open]')")
        assert page.locator('.flight-easter-egg input,.flight-easter-egg form').count() == 0
        assert page.evaluate("document.querySelector('#app').inert&&document.querySelector('main')===window.flightMain&&view===window.flightView&&projectId===window.flightProject")
        page.wait_for_function("landingFlight.snapshot().elapsed>80")
        # Modal containment must protect every workspace shortcut during the scene.
        page.keyboard.type("nq/e`")
        assert page.evaluate("view===window.flightView&&!document.querySelector('#editor[open]')")
        page.keyboard.press("Tab")
        assert page.evaluate("document.activeElement===document.querySelector('.flight-easter-egg .login-moon')")
        page.keyboard.press("ArrowLeft")
        assert page.evaluate("landingFlight.snapshot().moon.x<319")
        if screenshot:
            page.evaluate("landingFlight.seekPhase('lift',.7,0)")
            page.screenshot(path=str(screenshot))
        page.wait_for_function("Number(getComputedStyle(document.querySelector('.flight-easter-egg-shade')).opacity)>.98")
        page.keyboard.press("Escape")
        assert page.evaluate("!!flightEasterEgg?.closing&&document.querySelector('#app').inert")
        page.wait_for_function("()=>{const shade=document.querySelector('.flight-easter-egg-shade');return shade&&Number(getComputedStyle(shade).opacity)<.9}")
        page.wait_for_function("!flightEasterEgg&&!landingFlight")
        assert page.evaluate("!document.querySelector('.flight-easter-egg,.flight-easter-egg-shade')&&!document.querySelector('#app').inert&&document.querySelector('main')===window.flightMain&&document.activeElement===window.flightOpener&&view===window.flightView&&projectId===window.flightProject&&scrollY===window.flightScroll")
    # Codes entered while editing must remain ordinary text.
    page.evaluate("{const input=document.createElement('input');input.id='flight-edit-proof';document.querySelector('main').append(input);input.focus()}")
    page.keyboard.type("idkfa iddqd")
    assert page.locator('#flight-edit-proof').input_value() == "idkfa iddqd"
    assert page.locator('.flight-easter-egg').count() == 0
    page.evaluate("document.querySelector('#flight-edit-proof').remove();window.flightOpener.focus()")
    # Reduced motion opens and closes without motion or delayed disposal.
    page.emulate_media(reduced_motion="reduce")
    page.keyboard.type("iddqd")
    page.wait_for_function("!!landingFlight")
    assert page.evaluate("landingFlight.snapshot().phase==='rest'&&landingFlight.snapshot().engine===0")
    page.keyboard.press("Escape")
    assert page.evaluate("!flightEasterEgg&&!landingFlight&&!document.querySelector('#app').inert")
    page.emulate_media(reduced_motion="no-preference")
    page.set_viewport_size({"width": 390, "height": 844})
    page.keyboard.type("idkfa")
    page.wait_for_function("!!landingFlight")
    assert page.evaluate("document.documentElement.scrollWidth<=innerWidth")
    if screenshot:
        page.screenshot(path=str(screenshot.with_stem(screenshot.stem + "-mobile")))
    page.keyboard.press("Escape")
    page.wait_for_function("!flightEasterEgg&&!landingFlight")
    page.set_viewport_size({"width": 1280, "height": 800})
    # Session teardown must cancel the exit timeout before starting a new login scene.
    page.keyboard.type("idkfa")
    page.wait_for_function("!!landingFlight")
    page.keyboard.press("Escape")
    page.evaluate("login()")
    page.wait_for_function("!!landingFlight&&!!document.querySelector('#login')")
    page.wait_for_timeout(400)
    assert page.evaluate("!flightEasterEgg&&!!landingFlight&&!document.querySelector('.flight-easter-egg-shade')&&!document.querySelector('#app').inert")
    page.locator('#login [name="username"]').fill("")
    page.keyboard.type("iddqd")
    assert page.locator('.flight-easter-egg').count() == 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--worker", action="store_true", help=argparse.SUPPRESS)
    parser.add_argument("--screenshot", type=Path, default=Path("/tmp/teleport-flight-easter-egg.png"))
    args = parser.parse_args()
    if not args.worker:
        from browser_setup import browser_python
        subprocess.run([browser_python(), "-B", __file__, "--worker", "--screenshot", str(args.screenshot)], check=True, timeout=20)
        return
    from playwright.sync_api import sync_playwright
    account = {
        "account": {"id": "flight-proof", "username": "flight-proof", "administrator": True},
        "installationId": "isolated-flight-proof", "revision": 1, "contentRevision": 1,
        "preferences": {"version": 1, "style": "paper", "mode": "light", "defaultView": "Todos", "defaultProject": 0},
        "lastLocation": {"view": "Todos", "projectId": 0}, "settings": {"executionMode": "off"},
    }
    errors = []
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        try:
            page = browser.new_page(viewport={"width": 1280, "height": 800})
            page.set_default_timeout(2500)
            page.on("pageerror", lambda error: errors.append(str(error)))

            def serve(route):
                request = route.request
                path = urlparse(request.url).path
                if path.startswith("/api/tools/"):
                    operation = path.rsplit("/", 1)[-1]
                    if request.method != "POST":
                        route.fulfill(status=405)
                    elif operation == "getSession":
                        route.fulfill(json=account)
                    elif operation.startswith("list"):
                        route.fulfill(json={"items": []})
                    else:
                        errors.append("Unexpected API operation: " + operation)
                        route.fulfill(status=400, json={"error": {"message": "Unexpected fixture operation"}})
                    return
                file = (WEB / (path.lstrip("/") or "index.html")).resolve()
                if not file.is_relative_to(WEB) or not file.is_file():
                    route.fulfill(status=404)
                    return
                content_type = mimetypes.guess_type(file)[0] or "application/octet-stream"
                route.fulfill(body=file.read_bytes(), content_type=content_type)

            # localhost supplies the secure context required by the real dialog's UUIDs.
            page.route("http://localhost:14318/**", serve)
            page.goto("http://localhost:14318/", wait_until="domcontentloaded")
            page.locator('#nav-views [data-view="Todos"]').wait_for()
            verify_flight_easter_egg(page, args.screenshot)
            page.evaluate("landingFlight.seekPhase('outbound',.1,0)")
            assert page.locator('.landing-planet').count() == 8
            assert page.locator('.landing-star').count() >= 20
            page.evaluate("landingFlight.seekPhase('orbit',.5,0);window.flightBeforeMove=landingFlight.snapshot();document.querySelector('.login-moon').focus()")
            page.keyboard.press("ArrowLeft")
            assert page.evaluate("()=>{const current=landingFlight.snapshot();const old=window.flightBeforeMove;return current.moon.x<old.moon.x&&current.x===old.x&&current.y===old.y}")
            page.evaluate("landingFlight.simulate(50)")
            assert page.evaluate("()=>{const current=landingFlight.snapshot();const old=window.flightBeforeMove;const distance=Math.hypot(current.x-old.x,current.y-old.y);return distance>0&&distance<=4.5}")
            page.evaluate("landingFlight.seekPhase('outbound',.1,0)")
            maximum, held = 0, False
            for _ in range(35):
                state = page.evaluate("landingFlight.simulate(1000)")
                maximum = max(maximum, len(state["fleet"]))
                held = held or state["holding"] or any(craft.get("phase") == "holding" for craft in state["fleet"])
                assert len(state["fleet"]) <= 9, state
                pad_craft = [craft for craft in state["fleet"] if craft.get("phase") in ("lift", "descend")]
                assert len(pad_craft) <= 1, state
                if state["phase"] not in ("outbound", "orbit", "return"):
                    assert not pad_craft, state
            assert maximum >= 2, maximum
            assert held, "A returning rocket never entered the holding pattern"
            assert not errors, json.dumps(errors)
        finally:
            browser.close()
    print("PASS: flight codes, focus and fade, mobile/reduced motion, eight planets, stars, multiple rockets and pad coordination")


if __name__ == "__main__":
    from tool_history import run
    run(main)
