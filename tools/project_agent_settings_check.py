"""Check project agent settings in an isolated browser without a real harness."""
import json
import mimetypes
from pathlib import Path
import subprocess
import sys
from urllib.parse import urlparse

ROOT = Path(__file__).resolve().parents[1]
WEB = ROOT / 'internal/server/web'


def main():
    if '--worker' not in sys.argv:
        from browser_setup import browser_python
        subprocess.run([browser_python(), '-B', __file__, '--worker'], check=True, timeout=20)
        return
    from playwright.sync_api import sync_playwright
    project = {'id': 1, 'version': 1, 'name': 'Orbits', 'key': 'ORB', 'description': '', 'members': {'fixture': 'Maintainer'}, 'refineWorkers': 1, 'doingWorkers': 1, 'capacityConfigured': True}
    account = {'account': {'id': 'fixture', 'username': 'fixture', 'administrator': True}, 'installationId': 'fixture', 'revision': 1, 'contentRevision': 1, 'preferences': {'version': 1, 'style': 'paper', 'mode': 'light', 'defaultView': 'Backlog', 'defaultProject': 0}, 'lastLocation': {'view': 'Backlog', 'projectId': 0}, 'settings': {'executionMode': 'off'}}
    calls, errors = [], []
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        try:
            page = browser.new_page(viewport={'width': 1280, 'height': 800})
            page.set_default_timeout(3000)
            page.on('pageerror', lambda error: errors.append(str(error)))

            def serve(route):
                request = route.request
                path = urlparse(request.url).path
                if path.startswith('/api/tools/'):
                    operation = path.rsplit('/', 1)[-1]
                    data = request.post_data_json
                    calls.append((operation, data))
                    if operation == 'getSession': result = account
                    elif operation in ('listProjects', 'listProjectSummaries'): result = {'items': [project]}
                    elif operation == 'getLocalHarnesses': result = {'mode': 'local', 'available': [{'id': 'codex', 'title': 'Codex · native local', 'available': True}]}
                    elif operation == 'setLaneCapacity':
                        assert data['expectedVersion'] == project['version']
                        project.update(maxAgents=data['maxAgents'], refineWorkers=data['refineWorkers'], doingWorkers=data['doingWorkers'], allowedHarnesses=data['allowedHarnesses'], version=project['version']+1)
                        result = project
                    elif operation == 'getHome': result = {'recent': [], 'upcoming': [], 'inProgress': [], 'remaining': [], 'counts': {}}
                    elif operation == 'getBacklogOrder': result = {'version': 0, 'keys': []}
                    elif operation == 'saveCurrentLocation': result = data
                    elif operation.startswith('list'): result = {'items': []}
                    else:
                        errors.append('Unexpected operation: '+operation)
                        route.fulfill(status=400, json={'error': {'message': 'Unexpected fixture operation'}})
                        return
                    route.fulfill(json=result)
                    return
                file = (WEB / (path.lstrip('/') or 'index.html')).resolve()
                if not file.is_relative_to(WEB) or not file.is_file():
                    route.fulfill(status=404)
                    return
                route.fulfill(body=file.read_bytes(), content_type=mimetypes.guess_type(file)[0] or 'application/octet-stream')

            page.route('http://localhost:14318/**', serve)
            page.goto('http://localhost:14318/', wait_until='domcontentloaded')
            page.locator('#account').wait_for()
            page.evaluate('openProjectDirectory()')
            page.locator('[data-project-edit="1"]').wait_for()
            page.locator('[data-project-edit="1"]').click()
            page.locator('#directory-project-settings').click()
            page.locator('#project-max-agents').wait_for()
            page.locator('#save-project-agents:enabled').wait_for()
            page.locator('#project-max-agents').fill('3')
            page.locator('#project-refine-workers').fill('2')
            page.locator('#project-doing-workers').fill('1')
            page.locator('#project-all-harnesses').uncheck()
            assert page.locator('[data-project-harness="codex"]').is_checked()
            page.screenshot(path='/tmp/teleport-project-agent-settings.png')
            page.locator('#save-project-agents').click()
            page.wait_for_function("document.querySelector('#project-agent-status').textContent.includes('saved')")
            assert project['maxAgents'] == 3 and project['allowedHarnesses'] == ['codex']
            assert 'not available yet' in page.locator('#editor').inner_text()
            page.locator('#dismiss').click()
            page.locator('#account').click()
            page.locator('#account-system').click()
            page.locator('#refresh-harnesses').click()
            page.wait_for_function("document.querySelector('#harness-refresh-status').textContent.includes('Codex')")
            assert sum(operation == 'getLocalHarnesses' for operation, _ in calls) >= 2
            assert not errors, json.dumps(errors)
        finally:
            browser.close()
    print('PASS: project agent settings, supported harness policy and system refresh')


if __name__ == '__main__':
    from tool_history import run
    run(main)
