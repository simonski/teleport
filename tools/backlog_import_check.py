"""Exercise backlog text import with an isolated, no-model browser fixture."""
import json
import mimetypes
from pathlib import Path
import subprocess
import sys
from urllib.parse import urlparse

ROOT = Path(__file__).resolve().parents[1]
WEB = ROOT / "internal/server/web"


def main():
    if "--worker" not in sys.argv:
        from browser_setup import browser_python
        subprocess.run([browser_python(), "-B", __file__, "--worker"], check=True, timeout=20)
        return
    from playwright.sync_api import sync_playwright
    saved = []
    account = {"account": {"id": "import-fixture", "username": "fixture", "administrator": True}, "installationId": "import-fixture", "revision": 1, "contentRevision": 1, "preferences": {"version": 1, "style": "paper", "mode": "light", "defaultView": "Backlog", "defaultProject": 0}, "lastLocation": {"view": "Backlog", "projectId": 0}, "settings": {"executionMode": "off"}}
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
                    if operation == "getSession":
                        result = account
                    elif operation == "saveIdea":
                        value = request.post_data_json
                        idea = {**value, "id": "idea:1", "version": 1, "creator": "import-fixture", "placement": "inbox"}
                        saved.append(idea)
                        result = idea
                    elif operation == "getIdea":
                        result = {"idea": saved[-1], "proposals": [], "acceptances": [], "links": []}
                    elif operation == "getBacklogOrder":
                        result = {"version": 0, "keys": []}
                    elif operation.startswith("list") or operation == "getIdeaHistory":
                        result = {"items": []}
                    else:
                        errors.append("Unexpected operation: " + operation)
                        route.fulfill(status=400, json={"error": {"message": "Unexpected fixture operation"}})
                        return
                    route.fulfill(json=result)
                    return
                file = (WEB / (path.lstrip("/") or "index.html")).resolve()
                if not file.is_relative_to(WEB) or not file.is_file():
                    route.fulfill(status=404)
                    return
                route.fulfill(body=file.read_bytes(), content_type=mimetypes.guess_type(file)[0] or "application/octet-stream")

            page.route("http://localhost:14318/**", serve)
            page.goto("http://localhost:14318/", wait_until="domcontentloaded")
            page.locator('#backlog-import').wait_for()
            page.locator('#backlog-import').focus()
            page.keyboard.type('idkfa')
            page.locator('.flight-easter-egg[open]').wait_for()
            page.keyboard.press('Escape')
            page.wait_for_function("!flightEasterEgg&&!landingFlight")
            page.locator('#backlog-import').focus()
            page.keyboard.press('i')
            page.locator('#import-text').wait_for()
            assert 'not available yet' in page.locator('#editor').inner_text()
            page.locator('#import-text').fill('A thought')
            page.keyboard.type('i')
            assert page.locator('#import-text').input_value() == 'A thoughti'
            page.locator('#cancel').click()
            page.locator('#backlog-import').click()
            page.locator('#import-file').set_input_files({"name": "large.txt", "mimeType": "text/plain", "buffer": b"a" * 20001})
            assert 'exceeds' in page.locator('#import-error').inner_text() and not saved
            page.locator('#import-file').set_input_files({"name": "invalid.txt", "mimeType": "text/plain", "buffer": b"\xff"})
            page.wait_for_function("document.querySelector('#import-error').textContent.includes('UTF-8')")
            page.locator('#import-file').set_input_files({"name": "notes.txt", "mimeType": "text/plain", "buffer": b"A goal\n\nMore detail\n"})
            page.wait_for_function("document.querySelector('#import-text').value==='A goal\\n\\nMore detail\\n'")
            page.screenshot(path='/tmp/teleport-backlog-import.png')
            page.locator('#editor-form button[type=submit]').click()
            page.wait_for_function("document.querySelector('#editor')?.textContent.includes('idea:1')")
            assert len(saved) == 1 and saved[0]['text'] == 'A goal\n\nMore detail\n' and saved[0]['title'] == 'A goal' and saved[0]['projectId'] == 0
            assert not errors, json.dumps(errors)
        finally:
            browser.close()
    print('PASS: backlog import shortcut, UTF-8 upload, private capture and classifier availability')


if __name__ == '__main__':
    from tool_history import run
    run(main)
