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
    base = {"projectId": 1, "status": "Todo", "phase": "Todo", "workflow": "fixture@1", "agent": False, "version": 1}
    rows = [{**base, "id": 7, "parentId": 0, "kind": "goal", "title": "Fixture goal"}, {**base, "id": 8, "parentId": 7, "kind": "epic", "title": "First epic"}, {**base, "id": 9, "parentId": 7, "kind": "epic", "title": "Second epic"}, {**base, "id": 11, "parentId": 8, "kind": "task", "title": "Locked task", "status": "Doing"}, {**base, "id": 12, "projectId": 2, "parentId": 0, "kind": "epic", "title": "Other project epic"}, {**base, "id": 10, "parentId": 8, "kind": "task", "title": "Fixture task"}]
    calls = []
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
                    calls.append((operation, request.post_data_json))
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
                    elif operation == "listIdeas":
                        result = {"items": [idea for idea in saved if not idea.get('deleted')]}
                    elif operation == "listTickets":
                        result = {"items": [row for row in rows if not row.get('deleted')]}
                    elif operation == "getTicket":
                        result = next(row for row in rows if row['id'] == request.post_data_json['id'])
                    elif operation == "saveBacklogOrder":
                        move = request.post_data_json['move']
                        target = next(row for row in rows if row['id'] == move['id'])
                        assert move['expectedVersion'] == target['version'] and move['expectedParentVersion'] == 1
                        target['parentId'] = move['parentId']
                        target['version'] += 1
                        result = {"version": 1, "keys": request.post_data_json['keys']}
                    elif operation == "requestRefinement":
                        target = next(row for row in rows if row['id'] == request.post_data_json['id'])
                        assert request.post_data_json['expectedVersion'] == target['version']
                        target.update(status='Refine', phase='Refine', agent=True, version=target['version']+1)
                        result = target
                    elif operation == "setTicketVisibility":
                        target = next(row for row in rows if row['id'] == request.post_data_json['id'])
                        assert request.post_data_json['expectedVersion'] == target['version']
                        target.update(deleted=True, version=target['version']+1)
                        result = target
                    elif operation == "setIdeaVisibility":
                        target = next(row for row in saved if row['id'] == request.post_data_json['id'])
                        assert request.post_data_json['expectedVersion'] == target['version']
                        target.update(deleted=True, version=target['version']+1)
                        result = target
                    elif operation == "getWorkflow":
                        result = {"id": "fixture", "version": 1, "phases": [{"id": "Todo", "title": "Todo", "role": "Todo"}], "rules": []}
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
            assert len(saved) == 1 and saved[0]['text'] == 'A goal\n\nMore detail\n' and saved[0]['title'] == 'A goal' and saved[0]['projectId'] == 0 and saved[0]['origin'] == 'import'
            page.locator('#cancel').click()
            card = page.locator('[data-backlog-open="idea:1"]')
            card.wait_for()
            card.click(button='right')
            menu = page.locator('.ticket-context-menu')
            assert menu.get_by_role('menuitem', name='Breakdown').is_disabled()
            assert menu.get_by_role('menuitem', name='Spellcheck').is_disabled()
            assert 'not available yet' in menu.inner_text()
            page.keyboard.press('Escape')
            assert page.evaluate("document.activeElement===document.querySelector('[data-backlog-open=\"idea:1\"]')")
            page.keyboard.press('Shift+F10')
            assert page.locator('.ticket-context-menu').is_visible()
            page.screenshot(path='/tmp/teleport-backlog-context.png')
            page.keyboard.press('Escape')
            page.locator('[data-backlog-open="ticket:7"]').click(button='right')
            page.get_by_role('menuitem', name='Open ticket').wait_for()
            page.keyboard.press('Escape')
            page.locator('[data-backlog-open="ticket:11"]').click(button='right')
            assert page.get_by_role('menuitem', name='Assign to epic').is_disabled()
            assert page.get_by_role('menuitem', name='Refine').is_disabled()
            page.keyboard.press('Escape')
            page.locator('[data-backlog-open="ticket:10"]').click(button='right')
            menu = page.locator('.ticket-context-menu')
            assert menu.get_by_role('menuitem', name='Breakdown').is_disabled()
            assert menu.get_by_role('menuitem', name='Spellcheck').is_disabled()
            assign = menu.get_by_role('menuitem', name='Assign to epic')
            assign.focus()
            page.keyboard.press('ArrowRight')
            assert page.get_by_role('menu', name='Assign to epic').get_by_role('menuitem', name='First epic').is_disabled()
            assert page.get_by_role('menu', name='Assign to epic').get_by_role('menuitem', name='Other project epic').count() == 0
            page.screenshot(path='/tmp/teleport-backlog-epic-menu.png')
            page.get_by_role('menu', name='Assign to epic').get_by_role('menuitem', name='Second epic').click()
            page.wait_for_function("tickets.find(ticket=>ticket.id===10)?.parentId===9&&!backlogPending")
            assert rows[-1]['parentId'] == 9
            page.locator('[data-backlog-open="ticket:10"]').click(button='right')
            page.get_by_role('menuitem', name='Refine').click()
            page.wait_for_function("document.querySelector('[data-backlog-open=\"ticket:10\"]')?.dataset.status==='Refine'")
            page.locator('[data-backlog-open="ticket:10"]').click(button='right')
            page.get_by_role('menuitem', name='Delete').click()
            page.locator('#editor-form button[type=submit]').click()
            page.wait_for_function("!document.querySelector('[data-backlog-open=\"ticket:10\"]')")
            assert any(operation == 'setTicketVisibility' for operation, _ in calls)
            page.locator('[data-backlog-open="idea:1"]').click(button='right')
            page.get_by_role('menuitem', name='Delete').click()
            page.locator('#editor-form button[type=submit]').click()
            page.wait_for_function("!document.querySelector('[data-backlog-open=\"idea:1\"]')")
            assert saved[0]['deleted']
            assert not errors, json.dumps(errors)
        finally:
            browser.close()
    print('PASS: backlog import, context menus, scoped epic assignment, refinement and deletion')


if __name__ == '__main__':
    from tool_history import run
    run(main)
