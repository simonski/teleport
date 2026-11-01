"""Persistent Chromium acceptance driver with the existing browser helper interface."""
import json
import time


class Browser:
    def __init__(self, session, environment):
        from playwright.sync_api import sync_playwright
        self.session, self.environment = session, environment
        self.playwright = sync_playwright().start()
        self.connection, self.page = None, None
        self.errors = []

    def _click(self, locator):
        locator.wait_for(state="visible")
        locator.evaluate("element=>element.scrollIntoView({block:'center',inline:'nearest',behavior:'instant'})")
        # Dispatch an actual pointer click at the scrolled control. Each following
        # assertion observes the application result rather than waiting two
        # animation frames for every intermediate button's geometry to settle.
        try:
            locator.click(force=True)
        except Exception as error:
            # A synchronous view replacement can resolve a fresh control before
            # layout. Retry only a click rejected before pointer dispatch.
            if "Element is not visible" not in str(error):
                raise
            locator.click()

    def __call__(self, *arguments):
        command, *values = arguments
        if command == "open":
            self.connection = self.playwright.chromium.launch(headless=True, timeout=5000, args=[
                "--disable-background-timer-throttling", "--disable-renderer-backgrounding",
                "--disable-backgrounding-occluded-windows", "--enable-unsafe-swiftshader",
            ])
            self.page = self.connection.new_page(viewport={"width":1280,"height":800})
            self.protocol = self.page.context.new_cdp_session(self.page)
            self.page.set_default_timeout(2000)
            self.page.goto(values[0], wait_until="domcontentloaded")
            self.page.bring_to_front()
            self.page.on("pageerror", lambda error: self.errors.append(str(error)))
        elif command == "eval":
            # Runtime.evaluate preserves ordinary script completion values.
            # page.evaluate auto-invokes a returned function, which changes the
            # meaning of assignments used by the existing failure fixtures.
            result = self.protocol.send("Runtime.evaluate", {"expression":values[0], "awaitPromise":True, "returnByValue":True})
            if "exceptionDetails" in result:
                details = result["exceptionDetails"]
                raise RuntimeError(details.get("exception", {}).get("description", details.get("text", "Browser evaluation failed")))
            return json.dumps(result["result"].get("value"))
        elif command == "wait":
            if values[0] == "--fn":
                # Evaluate directly: Playwright's string predicate compiler uses
                # eval(), which the application's CSP deliberately prohibits.
                deadline = time.monotonic() + 2
                while not self.page.evaluate(values[1]):
                    if time.monotonic() >= deadline:
                        raise TimeoutError("Browser condition did not become true")
                    self.page.wait_for_timeout(10)
            else:
                self.page.locator(values[0]).first.wait_for(state="visible")
        elif command == "click":
            self._click(self.page.locator(values[0]))
        elif command == "fill":
            self.page.locator(values[0]).fill(values[1])
        elif command == "focus":
            self.page.locator(values[0]).focus()
        elif command == "scrollintoview":
            self.page.locator(values[0]).scroll_into_view_if_needed()
        elif command == "hover":
            self.page.locator(values[0]).hover()
        elif command == "drag":
            self.page.locator(values[0]).drag_to(self.page.locator(values[1]))
        elif command == "check":
            self.page.locator(values[0]).check()
        elif command == "get":
            if values[0] == "text":
                return self.page.locator(values[1]).inner_text()
            if values[0] == "value":
                return self.page.locator(values[1]).input_value()
            raise ValueError("Unsupported browser read")
        elif command == "press":
            self.page.keyboard.press(values[0])
        elif command == "find":
            kind, value, action, *options = values
            exact = "--exact" in options
            if kind == "role":
                name = options[options.index("--name") + 1] if "--name" in options else None
                locator = self.page.get_by_role(value, name=name, exact=exact)
            else:
                locator = getattr(self.page, "get_by_" + kind)(value, exact=exact)
            if action == "fill":
                locator.fill(options[0])
            elif action == "click":
                self._click(locator)
            else:
                getattr(locator, action)()
        elif command == "mouse":
            action, *coordinates = values
            if action == "move":
                self.page.mouse.move(float(coordinates[0]), float(coordinates[1]))
            else:
                getattr(self.page.mouse, action)(button=coordinates[0])
        elif command == "set":
            if values[0] == "viewport":
                self.page.set_viewport_size({"width": int(values[1]), "height": int(values[2])})
                self.page.evaluate("()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))")
            elif values[0] == "media":
                self.page.emulate_media(color_scheme=values[1], reduced_motion="reduce" if "reduced-motion" in values else "no-preference")
            else:
                raise ValueError("Unsupported browser setting")
        elif command == "screenshot":
            self.page.screenshot(path=values[0], timeout=2000)
        elif command == "reload":
            self.page.reload(wait_until="domcontentloaded")
        elif command == "snapshot":
            return self.page.locator("body").aria_snapshot(timeout=1000)
        elif command == "errors":
            return json.dumps(self.errors)
        elif command == "close":
            try:
                if self.connection:
                    self.connection.close()
            finally:
                self.playwright.stop()
        else:
            raise ValueError("Unsupported browser action: " + command)
        return ""
