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
            assert page.locator('.landing-planet').count() == 1
            assert page.locator('.login-moon').count() == 1
            assert page.locator('.landing-planet > circle:first-child').get_attribute('r') == '21'
            assert page.locator('.landing-star').count() >= 20
            page.evaluate("landingFlight.seekPhase('orbit',.2,0);landingFlight.simulate(1000)")
            assert page.evaluate("()=>{const m=document.querySelector('.flight-rocket').transform.baseVal.consolidate().matrix,moon=landingFlight.snapshot().moon;const actual=Math.atan2(m.b,m.a)*180/Math.PI,target=Math.atan2(m.f-moon.y,m.e-moon.x)*180/Math.PI+90;return Math.abs(((target-actual)%360+540)%360-180)<1}")
            page.evaluate("landingFlight.seekPhase('orbit',.5,0);window.flightBeforeMove=landingFlight.snapshot();document.querySelector('.login-moon').focus()")
            page.keyboard.press("ArrowLeft")
            assert page.evaluate("()=>{const current=landingFlight.snapshot();const old=window.flightBeforeMove;return current.moon.x<old.moon.x&&current.x===old.x&&current.y===old.y}")
            page.evaluate("landingFlight.simulate(50)")
            assert page.evaluate("()=>{const current=landingFlight.snapshot();const old=window.flightBeforeMove;const distance=Math.hypot(current.x-old.x,current.y-old.y);return distance>0&&distance<=4.5}")
            page.evaluate("landingFlight.seekPhase('outbound',.1,0)")
            maximum, held, launches, delivered, collected, different_moons = 0, False, set(), False, False, False
            for _ in range(160):
                state = page.evaluate("landingFlight.simulate(1000)")
                maximum = max(maximum, len(state["fleet"]))
                launches.update(craft["serial"] for craft in state["fleet"])
                held = held or state["holding"] or any(craft.get("phase") == "holding" for craft in state["fleet"])
                delivered = delivered or any(craft.get("phase") == "delivery" for craft in state["fleet"])
                collected = collected or any(craft.get("phase") == "pickup" for craft in state["fleet"])
                for craft in state["fleet"]:
                    if state["phase"] == "orbit" and craft.get("destination"):
                        assert craft["destination"] != page.evaluate("landingFlight.snapshot().destination"), state
                        different_moons = True
                    if craft.get("phase") == "delivery" and craft["groundTime"] < 2400 or craft.get("phase") == "pickup" and craft["groundTime"] > 3500:
                        serial = str(craft["serial"])
                        aligned = page.evaluate("serial=>{const rocket=document.querySelector('.fleet-rocket[data-rocket=\"'+serial+'\"]');const carrier=document.querySelector('.fleet-carrier[data-rocket=\"'+serial+'\"]');return Math.abs(rocket.getCTM().e-carrier.getCTM().e)<.01}", serial)
                        assert aligned, "A transported rocket detached from its carrier"
                        if args.screenshot:
                            page.screenshot(path=str(args.screenshot.with_stem(args.screenshot.stem + "-carrier")))
                orbiting = []
                if state["phase"] == "orbit" and not state["holding"]:
                    orbiting.append((state["position"], page.evaluate("landingFlight.snapshot().destination")))
                orbiting.extend((craft["position"], craft["destination"]) for craft in state["fleet"] if craft.get("phase") == "orbit")
                for position, destination in orbiting:
                    centre = page.evaluate("name=>{if(name==='moon')return landingFlight.snapshot().moon;const body=document.querySelector('[data-planet=\"'+name+'\"]');return {x:Number(body.dataset.x),y:Number(body.dataset.y)}}", destination)
                    radius = ((position["x"]-centre["x"])**2+(position["y"]-centre["y"])**2)**.5
                    other_positions = [p for p in [state["position"]] + [craft["position"] for craft in state["fleet"]] if p is not position]
                    if all((position["x"]-p["x"])**2+(position["y"]-p["y"])**2 > 55**2 for p in other_positions):
                        assert 24 <= radius <= 42, (destination, radius, state)
                assert state["minimum"] is None or state["minimum"] >= 29, state
                assert state["maxTurn"] <= 13.500001, state
                assert len(state["fleet"]) <= 1, state
                positions = [state["position"]] + [craft["position"] for craft in state["fleet"]]
                for index, first in enumerate(positions):
                    for second in positions[index+1:]:
                        assert (first["x"]-second["x"])**2+(first["y"]-second["y"])**2 >= 29**2, state
                pad_craft = [craft for craft in state["fleet"] if craft.get("phase") in ("lift", "descend")]
                assert len(pad_craft) <= 1, state
                if state["phase"] not in ("outbound", "orbit", "return"):
                    assert not pad_craft, state
            assert maximum == 1, maximum
            assert held, "A returning rocket never entered the holding pattern"
            assert delivered and collected, "A rocket skipped its carrier delivery or pickup"
            assert different_moons, "No second rocket was assigned the other moon during an orbit"
            assert len(launches) >= 3, "Rockets did not land and release the pad for later launches"
            assert not errors, json.dumps(errors)
        finally:
            browser.close()
    print("PASS: flight codes, focus and fade, mobile/reduced motion, two moons, orbital attitude, stars, rocket carriers and pad coordination")


if __name__ == "__main__":
    from tool_history import run
    run(main)
