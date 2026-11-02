"""Exercise the actual tk init/server/client path in a disposable installation."""
import argparse
from contextlib import nullcontext
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request
from navigation_browser import verify_navigation
from ticket_browser import verify_ticket_keyboard
from project_browser import verify_project_directory
from workflow_lanes_browser import verify_workflow_lanes
from shell_polish_browser import verify_landing, verify_profile_dialogs
from graph_browser import verify_graph, verify_camera_retained
from studio_browser import verify_studio
from smoke_timeout import TIMEOUT_SECONDS
from smoke_profile import Profile

ROOT = Path(__file__).resolve().parents[1]

def main(temporary_root=None):
    parser = argparse.ArgumentParser(description=__doc__, epilog=f"Total wall-clock budget: {TIMEOUT_SECONDS} seconds, including cleanup.")
    parser.add_argument("--browser", action="store_true")
    parser.add_argument("--browser-driver", choices=("persistent", "cli"), default="persistent", help="Keep the CLI driver for identical-workload comparisons")
    parser.add_argument("--browser-slice", choices=("core", "shell", "views"), help="Run one isolated part of the full browser suite")
    parser.add_argument("--shell-only", action="store_true", help="Verify only the shared shell and profile popups; requires --browser")
    parser.add_argument("--navigation-only", action="store_true", help="Verify shared shell and persisted navigation; requires --browser")
    parser.add_argument("--tickets-only", action="store_true", help="Verify ticket creation and board keyboard controls; requires --browser")
    parser.add_argument("--projects-only", action="store_true", help="Verify project directory totals and metadata; requires --browser")
    parser.add_argument("--graph-only", action="store_true", help="Verify the bundled graph renderer, persistence and fallback; requires --browser")
    parser.add_argument("--studio-only", action="store_true", help="Verify the 2D graph, Studio appearance and workflow map; requires --browser")
    parser.add_argument("--database", help="Empty disposable database owned by the parity runner")
    args = parser.parse_args()
    if (args.browser_slice or args.shell_only or args.navigation_only or args.tickets_only or args.projects_only or args.graph_only or args.studio_only) and not args.browser:
        parser.error("Focused UI checks require --browser")
    context = tempfile.TemporaryDirectory(prefix="teleport-smoke-") if temporary_root is None else nullcontext(str(temporary_root))
    with context as temporary:
        folder = Path(temporary)
        if args.browser and not args.browser_slice and not args.database and not any((args.shell_only,args.navigation_only,args.tickets_only,args.projects_only,args.graph_only,args.studio_only)):
            from smoke_suite import run_suite
            run_suite(__file__, folder)
            return
        focus = next((name for name in ("shell", "navigation", "tickets", "projects", "graph", "studio") if getattr(args, name + "_only")), "suite-" + args.browser_slice if args.browser_slice else "full")
        profile = Profile(("postgres" if args.database else "sqlite") + "/" + (f"{focus}-{args.browser_driver}" if args.browser else "api"), folder)
        profile.start("setup")
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            port = sock.getsockname()[1]
        if args.browser and args.browser_driver == "cli":
            (folder / "smoke-control.json").write_text(json.dumps({"browserSession": f"teleport-smoke-{port}"}))
        endpoint = f"http://127.0.0.1:{port}"
        environment = dict(os.environ, TELEPORT_DB=args.database or str(folder / "database.db"), TELEPORT_CONFIG=str(folder / "client.json"), TELEPORT_SERVER=endpoint)
        # Hidden test tabs must keep polling during long acceptance flows.
        # Chrome otherwise throttles chained timers after five minutes hidden.
        browser_environment = dict(os.environ, AGENT_BROWSER_ARGS=",".join(filter(None, [
            os.environ.get("AGENT_BROWSER_ARGS", ""), "--disable-background-timer-throttling",
            "--disable-renderer-backgrounding", "--disable-backgrounding-occluded-windows",
        ])))
        def tk(*arguments, input=None, success=True):
            result = profile.call("tk", lambda: subprocess.run([str(ROOT / "bin/tk"), *arguments], env=environment, input=input, text=True, capture_output=True, timeout=3))
            if success and result.returncode:
                raise RuntimeError(result.stderr)
            if not success and not result.returncode:
                raise AssertionError("operation unexpectedly succeeded")
            return result.stdout
        assert "Initialization complete" in tk("init")
        process = subprocess.Popen([str(ROOT / "bin/tk"), "serve", "--addr", f"127.0.0.1:{port}"], env=environment, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        try:
            deadline = time.monotonic() + 15
            while True:
                try:
                    with urllib.request.urlopen(endpoint + "/healthz", timeout=1) as response:
                        assert response.status == 200
                        server_version = json.loads(response.read())["version"]
                    break
                except OSError:
                    if time.monotonic() > deadline or process.poll() is not None:
                        raise RuntimeError("server failed to start")
                    time.sleep(0.05)
            tk("init", success=False)
            tk("login", "--password-stdin", input="password\n")
            session = json.loads(tk("getSession"))
            assert session["account"]["administrator"] and not session["settings"]["agents"]
            assert len(json.loads(tk("listWorkflows"))["items"]) == 2
            assert len(json.loads(tk("tools"))["items"]) >= 4
            assert "passwordHash" not in json.dumps(session)
            project = json.loads(tk("createProject", "--data", json.dumps({"name": "Smoke flight", "key": "smoke-flight", "requestId": "smoke-project"})))
            permission = json.loads(tk("getEffectivePermission", "--data", json.dumps({"projectId": project["id"], "adapter": "codex-structured", "command": ["git", "status"]})))
            assert not permission["allowed"] and not permission["executable"] and permission["capabilities"]["missingProofs"]
            ticket = json.loads(tk("createTicket", "--data", json.dumps({"projectId": project["id"], "title": "Pack the flight case", "agent": False, "requestId": "smoke-ticket"})))
            ticket = json.loads(tk("transitionTicket", "--data", json.dumps({"id": ticket["id"], "expectedVersion": ticket["version"], "target": "Doing"})))
            assert ticket["status"] == "Doing"
            if args.browser:
                seeded_browser = args.browser_slice in ("core", "views") or args.studio_only
                if seeded_browser:
                    setup = json.loads(tk("saveWelcomeSetup", "--data", json.dumps({"expectedVersion": 0, "step": 5, "projectId": project["id"], "harness": "codex", "attention": "quiet"})))
                    assert json.loads(tk("completeWelcomeSetup", "--data", json.dumps({"expectedVersion": setup["version"], "confirm": True})))["completed"]
                driver = None
                if args.browser_driver == "persistent":
                    from browser_driver import Browser
                    driver = Browser(f"teleport-smoke-{port}", browser_environment)
                def browser(*arguments):
                    if arguments[0] == "select":
                        browser("click", arguments[1] + " + .choice-button")
                        return browser("click", ".choice-list [data-value=" + json.dumps(arguments[2]) + "]")
                    if len(arguments)>=6 and arguments[:5] == ("find", "role", "button", "click", "--name") and arguments[5] in ("Settings", "System", "Project settings"):
                        browser("click", "#account")
                        return browser("click", {"Settings":"#account-settings", "System":"#account-system", "Project settings":"#account-project-settings"}[arguments[5]])
                    if driver is not None:
                        return profile.call("browser." + arguments[0], lambda: driver(*arguments))
                    if arguments[0] == "click" and len(arguments) == 2:
                        # Native-driver visibility can include a few clipped pixels;
                        # centre the actual control inside its scroll container.
                        expression = f"document.querySelector({json.dumps(arguments[1])})?.scrollIntoView({{block:'center',inline:'nearest'}})"
                        profile.call("browser.scroll", lambda: subprocess.run(["agent-browser", "--session", f"teleport-smoke-{port}", "eval", expression], env=browser_environment, text=True, capture_output=True, timeout=3, check=True))
                    result = profile.call("browser." + arguments[0], lambda: subprocess.run(["agent-browser", "--session", f"teleport-smoke-{port}", *arguments], env=browser_environment, text=True, capture_output=True, timeout=3))
                    if result.returncode:
                        raise RuntimeError(result.stderr or result.stdout)
                    return result.stdout
                try:
                    profile.start("landing")
                    browser("open", endpoint)
                    browser("wait", "--fn", "document.querySelector('#build-version')?.textContent===document.querySelector('meta[name=teleport-build-version]')?.content")
                    assert json.loads(browser("eval", "document.querySelector('#build-version').textContent")) == server_version
                    if not (seeded_browser or args.graph_only or args.navigation_only):
                        verify_landing(browser)
                    profile.start("login" if seeded_browser else "welcome")
                    browser("find", "label", "Username", "fill", "admin")
                    browser("find", "label", "Password", "fill", "password")
                    browser("find", "role", "button", "click", "--name", "Sign in", "--exact")
                    if not seeded_browser:
                        browser("wait", "#welcome-status")
                        browser("screenshot", "/tmp/teleport-welcome.png")
                        browser("press", "Escape")
                        assert not json.loads(tk("getWelcomeSetup"))["setup"]["completed"]
                        if args.graph_only:
                            profile.start("graph")
                            verify_graph(browser, tk, project["id"], ticket)
                            return
                        browser("click", "#account")
                        browser("click", "#account-welcome")
                        browser("click", '#editor button[type="submit"]')
                        browser("wait", '#editor [name="projectId"]')
                        browser("fill", '#editor [name="projectName"]', " smoke   FLIGHT ")
                        browser("wait", "--fn", "document.querySelector('#welcome-project-status')?.innerText.includes('Project name already exists')")
                        assert len(json.loads(tk("listProjects"))["items"]) == 1
                        browser("fill", '#editor [name="projectName"]', "Welcome project")
                        browser("wait", "--fn", "document.querySelector('#editor [name=projectKey]')?.value==='welc'")
                        browser("screenshot", "/tmp/teleport-welcome-project.png")
                        browser("click", '#editor button[type="submit"]')
                        browser("wait", '#editor [name="harness"]')
                        assert len(json.loads(tk("listProjects"))["items"]) == 1
                        browser("eval", "if(document.querySelector('[name=repository],[name=storageRoot],#welcome-grant'))throw new Error('Wizard still requests filesystem access')")
                        browser("press", "Escape")
                        browser("click", "#account")
                        browser("click", "#account-welcome")
                        browser("wait", '#editor [name="harness"]')
                        browser("select", '#editor [name="harness"]', "codex")
                        browser("click", '#editor button[type="submit"]')
                        browser("wait", '#editor [name="attention"]')
                        browser("select", '#editor [name="attention"]', "quiet")
                        browser("click", '#editor button[type="submit"]')
                        browser("wait", ".setup-summary")
                        assert len(json.loads(tk("listProjects"))["items"]) == 1
                        browser("eval", "{const original=window.fetch;let lose=true;window.welcomeTrace=[];window.fetch=async(...args)=>{const stack=new Error().stack;const response=await original(...args);if(/WelcomeSetup$/.test(String(args[0])))window.welcomeTrace.push({op:String(args[0]),input:JSON.parse(args[1].body),stack,status:response.status,result:await response.clone().json()});if(lose&&String(args[0]).endsWith('/completeWelcomeSetup')){lose=false;throw new Error('Simulated lost completion response');}return response;};}")
                        browser("click", '#editor button[type="submit"]')
                        browser("wait", "--fn", "document.querySelector('#welcome-status')?.innerText.includes('Simulated lost completion response')")
                        assert len(json.loads(tk("listProjects"))["items"]) == 2
                        browser("click", '#editor button[type="submit"]')
                        browser("wait", "--fn", "!document.querySelector('tk-dialog[open]')")
                        setup = json.loads(tk("getWelcomeSetup"))["setup"]
                        assert setup["completed"] and setup["attention"] == "quiet"
                        assert len(json.loads(tk("listProjects"))["items"]) == 2
                        assert not json.loads(tk("getSession"))["settings"]["agents"]
                    browser("wait", "#project + .choice-button")
                    browser("select", "#project", str(project["id"]))
                    browser("wait", "--fn", "document.body.innerText.includes('Latest work')")
                    if args.studio_only:
                        profile.start("studio")
                        verify_studio(browser,tk,project,ticket,profile.start)
                        return
                    if args.navigation_only:
                        profile.start("navigation")
                        verify_navigation(browser, tk, project["id"])
                        return
                    if args.browser_slice not in ("core", "views"):
                        profile.start("shell")
                        browser("eval", "if(document.querySelector('main h1,.board-heading>.eyebrow')||Math.abs(document.querySelector('.topbar').getBoundingClientRect().height-59)>1)throw new Error('Shell is not compact');if(!document.querySelector('#javascript-console')||!document.querySelector('#harness-console'))throw new Error('Missing console buttons')")
                        browser("screenshot", "/tmp/teleport-shell.png")
                        verify_profile_dialogs(browser)
                        browser("find", "role", "button", "click", "--name", "Settings", "--exact")
                        browser("eval", "if(document.querySelector('#editor h2')?.textContent!=='Settings'||document.querySelector('#execution-mode,#configure-repository,#edit-workflows')||document.querySelector('[aria-current=page]')?.dataset.view!=='Home')throw new Error('Settings is not a personal popup over Home')")
                        browser("screenshot", "/tmp/teleport-settings.png")
                        browser("press", "Escape")
                        browser("eval", "if(document.activeElement!==document.querySelector('#account'))throw new Error('Settings dismissal lost usable focus')")
                        browser("find", "role", "button", "click", "--name", "System", "--exact")
                        browser("eval", "if(!document.querySelector('#execution-mode')||document.querySelector('#appearance-style,#configure-repository'))throw new Error('System mixes personal/project configuration')")
                        browser("click", "#system-brake")
                        browser("click", '#editor button[type="submit"]')
                        browser("wait", "--fn", "!document.querySelector('tk-dialog[open]')")
                        assert json.loads(tk("getSession"))["settings"]["handbrake"]
                        browser("find", "role", "button", "click", "--name", "System", "--exact")
                        browser("click", "#system-brake")
                        browser("press", "Escape")
                        assert json.loads(tk("getSession"))["settings"]["handbrake"]
                        browser("find", "role", "button", "click", "--name", "System", "--exact")
                        browser("click", "#system-brake")
                        browser("click", '#editor button[type="submit"]')
                        browser("wait", "--fn", "!document.querySelector('tk-dialog[open]')")
                        assert not json.loads(tk("getSession"))["settings"]["handbrake"]
                        browser("click", "#javascript-console")
                        browser("wait", "#script-source")
                        browser("press", "Escape")
                        assert not json.loads(tk("listScriptRuns"))["items"]
                        browser("click", "#harness-console")
                        browser("wait", "#new-console")
                        browser("press", "Escape")
                        assert not json.loads(tk("listSessions"))["items"]
                        browser("set", "viewport", "390", "844")
                        browser("eval", "if(document.documentElement.scrollWidth>innerWidth)throw new Error('Mobile shell overflows');if(!document.querySelector('#build-version').textContent)throw new Error('Missing mobile version')")
                        browser("screenshot", "/tmp/teleport-shell-mobile.png")
                        browser("set", "viewport", "1280", "800")
                        if args.shell_only:
                            print("PASS: real build footer, compact desktop/mobile shell, separate profile popups, exact confirmed system handbrake and console viewers without execution")
                            return
                        if args.tickets_only:
                            verify_ticket_keyboard(browser, tk, project["id"])
                            return
                        if args.projects_only:
                            verify_project_directory(browser, tk, project["id"])
                            return
                        profile.start("navigation")
                        verify_navigation(browser, tk, project["id"])
                        if args.navigation_only:
                            return
                        profile.start("tickets")
                        verify_ticket_keyboard(browser, tk, project["id"])
                        profile.start("projects")
                        verify_project_directory(browser, tk, project["id"])
                    if args.browser_slice == "shell":
                        return
                    if args.browser_slice != "views":
                        profile.start("ticket-delivery")
                        browser("eval", "if(document.querySelector('dialog,[data-view=Settings],#finder-open')||document.body.innerText.includes('Work, in motion'))throw new Error('Obsolete navigation or native dialog');if(!document.querySelector('.wordmark .rocket'))throw new Error('Missing rocket')")
                        browser("click", "#project + .choice-button")
                        browser("press", "ArrowDown")
                        browser("press", "Escape")
                        browser("eval", "if(document.querySelector('.choice-list')||document.activeElement!==document.querySelector('#project + .choice-button'))throw new Error('Choice keyboard dismissal lost focus')")
                        browser("click", "#new")
                        browser("fill", '[name="title"]', "Unsaved dismissal check")
                        browser("eval", "document.querySelector('#editor button[type=submit]').focus()")
                        browser("press", "Tab")
                        browser("eval", "if(document.activeElement!==document.querySelector('#dismiss'))throw new Error('Modal focus escaped')")
                        browser("press", "Escape")
                        browser("wait", "--fn", "!document.querySelector('tk-dialog[open]')&&!document.querySelector('#app').inert")
                        browser("click", "#new")
                        browser("mouse", "move", "3", "3")
                        browser("mouse", "down", "left")
                        browser("mouse", "up", "left")
                        browser("wait", "--fn", "!document.querySelector('tk-dialog[open]')")
                        browser("find", "role", "button", "click", "--name", "Todos", "--exact")
                        browser("find", "role", "button", "click", "--name", "Pack the flight case")
                        browser("wait", "--fn", "document.querySelector('tk-dialog')?.open")
                        browser("find", "role", "button", "click", "--name", "Move to Done", "--exact")
                        browser("find", "label", "Accept these completion criteria", "check")
                        browser("find", "role", "button", "click", "--name", "Complete ticket", "--exact")
                        browser("wait", "--fn", "!document.querySelector('tk-dialog')?.open")
                        assert json.loads(tk("getTicket", "--data", json.dumps({"id": ticket["id"]})))["status"] == "Done"
                        browser("screenshot", "/tmp/teleport-smoke.png")
                        draft = json.loads(tk("copyWorkflow", "--data", json.dumps({"sourceKey": "human@1", "projectId": project["id"], "title": "Flight workflow", "requestId": "smoke-flow"})))
                        profile.start("workflow")
                        browser("select", "#project", str(project["id"]))
                        browser("find", "role", "button", "click", "--name", "Project settings", "--exact")
                        browser("find", "role", "button", "click", "--name", "Workflow editor", "--exact")
                        browser("wait", "#flow-select")
                        browser("eval", "if(view!=='Workflows'||!document.querySelector('main #workflow-editor')||document.querySelector('tk-dialog[open]'))throw new Error('Workflow editor is not in its own view')")
                        browser("select", "#flow-select", draft["id"] + "@draft")
                        browser("fill", '[data-phase="1"] [data-field="title"]', "Discard this local title")
                        browser("click", '#nav-views [data-view="Home"]')
                        browser("eval", "if(view!=='Workflows'||document.querySelector('[data-phase=\"1\"] [data-field=title]').value!=='Discard this local title')throw new Error('Navigation discarded unsaved workflow input')")
                        browser("click", "#flow-discard")
                        browser("wait", "--fn", "!workflowLoading&&document.querySelector('[data-phase=\"1\"] [data-field=title]')?.value==='Doing'")
                        browser("fill", '[data-phase="1"] [data-field="title"]', "In flight")
                        browser("click", "#flow-save")
                        browser("wait", "--fn", "document.querySelector('#flow-message')?.innerText.includes('Draft saved')")
                        saved_revision = json.loads(tk("getSession"))["revision"]
                        browser("wait", "--fn", f"session.revision>={saved_revision}&&!workflowLoading&&document.querySelector('#flow-message')?.textContent.includes('Draft saved')")
                        browser("select", "#preview-source", "Doing")
                        browser("fill", "#preview-outcome", "return")
                        browser("click", "#flow-preview")
                        browser("wait", "--fn", "document.querySelector('#preview-result')?.innerText.includes('Todo')")
                        browser("eval", "window.scrollTo(0,0)")
                        browser("screenshot", "/tmp/teleport-workflow.png")
                        browser("eval", "if(!document.querySelector('#flow-activate').disabled)throw new Error('Unpublished draft can be selected without publication')")
                        browser("eval", "{const original=window.fetch;let lose=true;window.fetch=async(...args)=>{const response=await original(...args);if(lose&&String(args[0]).endsWith('/publishWorkflow')){lose=false;throw new Error('Simulated lost publication response');}return response;};}")
                        browser("click", "#flow-publish")
                        browser("wait", "--fn", "document.querySelector('#flow-message')?.textContent.includes('Simulated lost publication response')")
                        browser("eval", "if(!workflowPublicationRecovery||flow.published)throw new Error('Unconfirmed publication did not preserve its exact retry context')")
                        browser("click", "#flow-publish")
                        browser("wait", "--fn", "document.querySelector('#flow-message')?.textContent.includes('Published v-1')")
                        unchanged = next(item for item in json.loads(tk("listProjects"))["items"] if item["id"] == project["id"])
                        assert unchanged["humanWorkflow"] == "human@1"
                        publications = [item for item in json.loads(tk("listWorkflows", "--data", json.dumps({"projectId": project["id"]})))["items"] if item["id"] == draft["id"]]
                        assert len(publications) == 1 and publications[0]["published"] and publications[0]["version"] == 1
                        browser("screenshot", "/tmp/teleport-workflow-published.png")
                        browser("click", "#flow-activate")
                        browser("click", '#editor button[type="submit"]')
                        browser("wait", "--fn", "!document.querySelector('tk-dialog[open]')")
                        new_ticket = json.loads(tk("createTicket", "--data", json.dumps({"projectId": project["id"], "title": "Fresh workflow pin", "agent": False, "requestId": "new-pin"})))
                        assert new_ticket["workflow"] == draft["id"] + "@1"
                        assert json.loads(tk("getTicket", "--data", json.dumps({"id": ticket["id"]})))["workflow"] == "human@1"
                        browser("wait", "#flow-draft")
                        browser("click", "#flow-draft")
                        browser("wait", "--fn", "!workflowMutation&&!flow.published&&flow.version===2")
                        browser("fill", '[data-phase="1"] [data-field="title"]', "Retain after cancelled deletion")
                        browser("click", "#flow-delete")
                        browser("press", "Escape")
                        browser("eval", "if(document.querySelector('[data-phase=\"1\"] [data-field=title]').value!=='Retain after cancelled deletion')throw new Error('Closing confirmation discarded the editor')")
                        browser("click", "#flow-delete")
                        browser("click", '#editor button[type="submit"]')
                        browser("wait", "--fn", "!document.querySelector('tk-dialog[open]')&&!workflowLoading")
                        retained_flows = [item for item in json.loads(tk("listWorkflows", "--data", json.dumps({"projectId": project["id"]})))["items"] if item["id"] == draft["id"]]
                        assert len(retained_flows) == 1 and retained_flows[0]["published"] and retained_flows[0]["version"] == 1
                        browser("set", "viewport", "390", "844")
                        browser("eval", "if(document.documentElement.scrollWidth>innerWidth)throw new Error('Workflow editor overflows mobile page')")
                        browser("screenshot", "/tmp/teleport-workflows-mobile.png")
                        browser("set", "viewport", "1280", "800")
                        browser("find", "role", "button", "click", "--name", "Settings", "--exact")
                        browser("select", "#preference-view", "Workflows")
                        browser("select", "#preference-project", str(project["id"]))
                        browser("wait", "--fn", "!preferenceSaving&&!Object.keys(preferenceDraft).length")
                        browser("press", "Escape")
                        browser("reload")
                        browser("wait", "--fn", "view==='Workflows'&&!workflowLoading&&!!document.querySelector('#flow-select')")
                        browser("screenshot", "/tmp/teleport-workflows-view.png")
                        verify_workflow_lanes(browser, tk, project)
                        browser("find", "role", "button", "click", "--name", "Settings", "--exact")
                        browser("select", "#preference-view", "Home")
                        browser("select", "#preference-project", "0")
                        browser("wait", "--fn", "!preferenceSaving&&!Object.keys(preferenceDraft).length")
                        browser("press", "Escape")
                        agent_ticket = json.loads(tk("createTicket", "--data", json.dumps({"projectId": project["id"], "title": "Demonstrate supervised delivery", "requestId": "agent-ticket"})))
                        configuration = json.loads(tk("getSession"))["settings"]
                        tk("setExecutionMode", "--data", json.dumps({"mode": "demo", "expectedVersion": configuration["version"]}))
                        tk("requestRefinement", "--data", json.dumps({"id": agent_ticket["id"], "expectedVersion": agent_ticket["version"]}))
                        def wait_ticket(predicate):
                            deadline = time.monotonic() + 12
                            while time.monotonic() < deadline:
                                current = json.loads(tk("getTicket", "--data", json.dumps({"id": agent_ticket["id"]})))
                                if predicate(current):
                                    return current
                                time.sleep(0.1)
                            raise AssertionError("agent ticket did not reach expected state")
                        wait_ticket(lambda current: current.get("refinement") is not None)
                        browser("find", "role", "button", "click", "--name", "Board", "--exact")
                        browser("wait", "--fn", "document.body.innerText.includes('Demonstrate supervised delivery')")
                        browser("find", "role", "button", "click", "--name", "Demonstrate supervised delivery")
                        browser("wait", '[data-agent="accept"]')
                        browser("click", '[data-agent="accept"]')
                        browser("find", "label", "Accept this exact scope and basis", "check")
                        browser("find", "role", "button", "click", "--name", "Accept scope", "--exact")
                        browser("wait", "--fn", "!document.querySelector('tk-dialog[open]')")
                        candidate = wait_ticket(lambda current: current["status"] == "Approval")
                        assert candidate["attempts"] == 3 and candidate["agentReview"]["simulated"]
                        tk("approveTicket", "--data", json.dumps({"id": candidate["id"], "expectedVersion": candidate["version"], "confirm": True}), success=False)
                        configuration = json.loads(tk("getSession"))["settings"]
                        tk("setExecutionMode", "--data", json.dumps({"mode": "off", "expectedVersion": configuration["version"]}))
                        repository = folder / "repository"
                        profile.start("git-delivery")
                        repository.mkdir()
                        git_environment = dict(os.environ, GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull, GIT_AUTHOR_NAME="Smoke", GIT_AUTHOR_EMAIL="smoke@localhost", GIT_COMMITTER_NAME="Smoke", GIT_COMMITTER_EMAIL="smoke@localhost")
                        def git(path, *arguments):
                            subprocess.run(["git", "-c", f"core.hooksPath={os.devnull}", "-c", "commit.gpgsign=false", "-C", str(path), *arguments], env=git_environment, check=True, capture_output=True, timeout=30)
                        git(repository, "init", "-b", "main")
                        (repository / "README.md").write_text("Original fixture\n")
                        git(repository, "add", ".")
                        git(repository, "commit", "-m", "Initial")
                        project = next(item for item in json.loads(tk("listProjects"))["items"] if item["id"] == project["id"])
                        external = json.loads(tk("copyWorkflow", "--data", json.dumps({"sourceKey": "agent@1", "projectId": project["id"], "title": "Explicit external delivery", "requestId": "external-flow"})))
                        external["phases"] = [phase for phase in external["phases"] if phase["role"] != "Review"]
                        external["rules"] = [rule for rule in external["rules"] if rule["source"] != "Review"]
                        for rule in external["rules"]:
                            if rule.get("target") == "Review":
                                rule["target"] = "Approval"
                        external = json.loads(tk("saveWorkflow", "--data", json.dumps({"definition": external, "expectedRevision": external["draftRevision"]})))
                        project = json.loads(tk("activateWorkflowVersion", "--data", json.dumps({"workflowKey": external["id"] + "@draft", "expectedRevision": external["draftRevision"], "projectId": project["id"], "expectedProjectVersion": project["version"]})))
                        project = json.loads(tk("configureLocalRepository", "--data", json.dumps({"projectId": project["id"], "expectedVersion": project["version"], "repository": str(repository), "storageRoot": str(folder / "worktrees")})))
                        real = json.loads(tk("createTicket", "--data", json.dumps({"projectId": project["id"], "title": "Real external fixture", "requestId": "real-ticket"})))
                        configuration = json.loads(tk("getSession"))["settings"]
                        tk("setExecutionMode", "--data", json.dumps({"mode": "demo", "expectedVersion": configuration["version"]}))
                        tk("requestRefinement", "--data", json.dumps({"id": real["id"], "expectedVersion": real["version"]}))
                        agent_ticket = real
                        refined = wait_ticket(lambda current: current.get("refinement") is not None)
                        configuration = json.loads(tk("getSession"))["settings"]
                        tk("setExecutionMode", "--data", json.dumps({"mode": "off", "expectedVersion": configuration["version"]}))
                        tk("acceptRefinement", "--data", json.dumps({"id": real["id"], "expectedVersion": refined["version"], "proposalId": refined["refinement"]["id"]}))
                        browser("wait", "--fn", "document.body.innerText.includes('Real external fixture')")
                        browser("click", f'[data-ticket="{real["id"]}"]')
                        browser("wait", '[data-delivery="prepare"]')
                        browser("click", '[data-delivery="prepare"]')
                        browser("find", "role", "button", "click", "--name", "Prepare worktree", "--exact")
                        browser("wait", "--fn", "!document.querySelector('tk-dialog[open]')")
                        prepared = wait_ticket(lambda current: current.get("delivery", {}).get("worktree", {}).get("path"))
                        worktree = Path(prepared["delivery"]["worktree"]["path"])
                        (worktree / "README.md").write_text("Delivered fixture\n")
                        git(worktree, "add", ".")
                        git(worktree, "commit", "-m", "Deliver external fixture")
                        browser("click", f'[data-ticket="{real["id"]}"]')
                        browser("wait", '[data-delivery="report"]')
                        browser("click", '[data-delivery="report"]')
                        browser("find", "role", "button", "click", "--name", "Capture candidate", "--exact")
                        browser("wait", "--fn", "!document.querySelector('tk-dialog[open]')")
                        browser("click", f'[data-ticket="{real["id"]}"]')
                        browser("wait", '[data-delivery="approve"]')
                        browser("click", '[data-delivery="approve"]')
                        browser("find", "label", "Approve this exact code and completion basis", "check")
                        browser("find", "role", "button", "click", "--name", "Approve and fast-forward main", "--exact")
                        browser("wait", "--fn", "!document.querySelector('tk-dialog[open]')")
                        done = wait_ticket(lambda current: current["status"] == "Done")
                        assert done.get("agentReview") is None
                        assert (repository / "README.md").read_text() == "Delivered fixture\n"
                        browser("screenshot", "/tmp/teleport-delivery.png")
                    if args.browser_slice == "core":
                        return
                    profile.start("ideas")
                    browser("find", "role", "button", "click", "--name", "Ideas", "--exact")
                    browser("click", "#new-idea")
                    browser("find", "label", "Title", "fill", "A calmer boarding process")
                    browser("find", "label", "Idea", "fill", "Keep the initial thought while creating a separate plan.", "--exact")
                    browser("find", "role", "button", "click", "--name", "Create idea", "--exact")
                    browser("wait", "#idea-propose")
                    browser("click", "#idea-propose")
                    browser("fill", '[name="title"]', "Prepare the boarding checklist")
                    browser("select", '[name="intent"]', "human")
                    browser("find", "role", "button", "click", "--name", "Save proposal", "--exact")
                    browser("wait", "[data-accept-proposal]")
                    browser("click", "[data-accept-proposal]")
                    browser("find", "label", "Create or link this exact proposed work", "check")
                    browser("find", "role", "button", "click", "--name", "Accept proposal", "--exact")
                    browser("wait", ".accepted-label")
                    browser("screenshot", "/tmp/teleport-ideas.png")
                    source_idea = json.loads(tk("listIdeas", "--data", json.dumps({"projectId": project["id"]})))["items"][0]
                    assert source_idea["title"] == "A calmer boarding process" and source_idea["acceptedCount"] == 1
                    targets = json.loads(tk("getIdeaLinks", "--data", json.dumps({"id": source_idea["id"]})))["items"]
                    idea_ticket = json.loads(tk("getTicket", "--data", json.dumps({"id": targets[0]["ticketId"]})))
                    assert idea_ticket["title"] == "Prepare the boarding checklist" and idea_ticket["status"] == "Todo"
                    browser("click", "#dismiss")
                    profile.start("graph")
                    browser("find", "role", "button", "click", "--name", "Graph", "--exact")
                    browser("click", "#graph-3d")
                    browser("wait", "--fn", "document.querySelector('#graph-state')?.innerText.includes('entities')")
                    browser("click", f'[data-node="ticket:{idea_ticket["id"]}"]')
                    browser("fill", '[data-coordinate="x"]', "125")
                    browser("find", "role", "button", "click", "--name", "Apply position", "--exact")
                    browser("click", "#graph-save")
                    browser("wait", "--fn", "document.querySelector('#graph-state')?.innerText.includes('saved')")
                    graph = json.loads(tk("getGraph", "--data", json.dumps({"projectId": project["id"]})))
                    assert graph["layout"]["positions"][f'ticket:{idea_ticket["id"]}']["x"] == 125
                    assert json.loads(tk("getTicket", "--data", json.dumps({"id": idea_ticket["id"]})))["version"] == idea_ticket["version"]
                    browser("eval", "window.__graphCamera=JSON.stringify(graphRuntime.scene.snapshot())")
                    changed_ticket = json.loads(tk("updateTicket", "--data", json.dumps({"id": idea_ticket["id"], "expectedVersion": idea_ticket["version"], "priority": "high"})))
                    browser("wait", "--fn", f'tickets.some(t=>t.id==={idea_ticket["id"]}&&t.version==={changed_ticket["version"]})&&!!graphRuntime')
                    verify_camera_retained(browser)
                    browser("eval", "window.scrollTo(0,0)")
                    browser("screenshot", "/tmp/teleport-graph.png")
                    browser("find", "role", "button", "click", "--name", "Settings", "--exact")
                    browser("select", "#appearance-style", "instrument")
                    browser("select", "#appearance-mode", "dark")
                    browser("wait", "--fn", "document.documentElement.dataset.mode==='dark'&&!preferenceSaving&&!Object.keys(preferenceDraft).length")
                    prefs = json.loads(tk("getViewPreferences"))
                    assert prefs["style"] == "instrument" and prefs["mode"] == "dark"
                    browser("eval", "if(document.querySelector('#save-appearance,#save-navigation,#save-execution-mode')||document.querySelector('#notice').textContent.includes('appearance saved'))throw new Error('Obsolete preference save UI')")
                    browser("press", "Escape")
                    browser("find", "role", "button", "click", "--name", "Project settings", "--exact")
                    browser("click", "#inspect-native")
                    browser("wait", "--fn", "document.querySelector('tk-dialog[open]')?.innerText.includes('fresh server authorization')")
                    browser("click", '#editor button[type="submit"]')
                    browser("find", "role", "button", "click", "--name", "Graph", "--exact")
                    browser("wait", "--fn", "document.querySelector('#graph-state')?.innerText.includes('entities')")
                    browser("screenshot", "/tmp/teleport-graph-dark.png")
                    browser("set", "viewport", "390", "844")
                    browser("screenshot", "/tmp/teleport-mobile.png")
                    browser("set", "viewport", "1280", "800")
                    browser("find", "role", "button", "click", "--name", "Cost", "--exact")
                    browser("eval", "window.__nativeContext=HTMLCanvasElement.prototype.getContext;HTMLCanvasElement.prototype.getContext=function(kind,...args){return kind.startsWith('webgl')?null:window.__nativeContext.call(this,kind,...args)}")
                    browser("find", "role", "button", "click", "--name", "Graph", "--exact")
                    browser("wait", "--fn", "document.querySelector('#graph-stage')?.innerText.includes('WebGL is unavailable')")
                    browser("wait", f'[data-node="ticket:{idea_ticket["id"]}"]')
                    browser("click", f'[data-node="ticket:{idea_ticket["id"]}"]')
                    browser("wait", "#graph-apply-position")
                    browser("eval", "HTMLCanvasElement.prototype.getContext=window.__nativeContext")
                    browser("screenshot", "/tmp/teleport-graph-fallback.png")
                    profile.start("goals-calendar")
                    browser("find", "role", "button", "click", "--name", "Goals", "--exact")
                    browser("click", "#new-hierarchy")
                    browser("find", "label", "Title", "fill", "Reduce boarding delay")
                    browser("find", "role", "button", "click", "--name", "Create ticket", "--exact")
                    browser("wait", "--fn", "!document.querySelector('tk-dialog[open]')")
                    goals = json.loads(tk("getGoalView", "--data", json.dumps({"projectId": project["id"]})))["items"]
                    goal_id = next(g["id"] for g in goals if g["title"] == "Reduce boarding delay")
                    browser("wait", f'[data-ticket="{goal_id}"]')
                    browser("click", f'[data-ticket="{goal_id}"]')
                    browser("wait", "#goal-configure")
                    browser("click", "#goal-configure")
                    browser("wait", "#add-objective")
                    browser("click", "#add-objective")
                    browser("fill", '[data-objective="0"] [data-field="title"]', "Median delay")
                    browser("select", '[data-objective="0"] [data-field="direction"]', "atMost")
                    browser("fill", '[data-objective="0"] [data-field="target"]', "5")
                    browser("fill", '[data-objective="0"] [data-field="unit"]', "minutes")
                    browser("eval", "document.querySelector('#editor button[type=submit]').scrollIntoView({block:'center'})")
                    browser("find", "role", "button", "click", "--name", "Save goal configuration", "--exact")
                    browser("wait", "#goal-review")
                    browser("click", "#goal-review")
                    browser("eval", "{const date=document.querySelector('#goal-review-date');date.value='2026-01-31';date.dispatchEvent(new Event('input',{bubbles:true}));date.dispatchEvent(new Event('change',{bubbles:true}));if(date.value!=='2026-01-31')throw new Error('Review date was not set')}")
                    browser("fill", "#goal-review-note", "Delivery alone does not prove less waiting.")
                    browser("eval", "document.querySelector('#editor button[type=submit]').scrollIntoView({block:'center'})")
                    browser("find", "role", "button", "click", "--name", "Save human review", "--exact")
                    browser("wait", ".goal-review")
                    browser("screenshot", "/tmp/teleport-goal-review.png")
                    goals = json.loads(tk("getGoalView", "--data", json.dumps({"projectId": project["id"]})))["items"]
                    reviewed_goal = next(g for g in goals if g["title"] == "Reduce boarding delay")
                    reviews = json.loads(tk("listGoalReviews", "--data", json.dumps({"id": reviewed_goal["id"]})))["items"]
                    assert reviews[0]["assessment"] == "unknown" and reviews[0]["nextReview"] == "2026-02-28"
                    browser("click", "#dismiss")
                    browser("press", "Shift")
                    browser("press", "Shift")
                    browser("fill", "#finder-query", f'#{idea_ticket["id"]}')
                    browser("wait", "[data-result]")
                    browser("screenshot", "/tmp/teleport-finder.png")
                    browser("click", '#editor button[type="submit"]')
                    browser("wait", '[name="start"]')
                    browser("eval", "for(const [name,value]of Object.entries({start:'2026-03-28',eta:'2026-03-30'})){const input=document.querySelector(`[name=${name}]`);input.value=value;input.dispatchEvent(new Event('input',{bubbles:true}));input.dispatchEvent(new Event('change',{bubbles:true}));}")
                    browser("eval", "document.querySelector('#editor button[type=submit]').scrollIntoView({block:'center'})")
                    browser("find", "role", "button", "click", "--name", "Save changes", "--exact")
                    browser("wait", "--fn", "!document.querySelector('tk-dialog[open]')")
                    browser("find", "role", "button", "click", "--name", "Calendar", "--exact")
                    browser("wait", "#calendar-controls")
                    browser("select", '#calendar-controls [name="period"]', "week")
                    browser("eval", "{const date=document.querySelector('#calendar-controls [name=date]');date.value='2026-03-29';date.dispatchEvent(new Event('input',{bubbles:true}));}")
                    browser("find", "role", "button", "click", "--name", "Apply calendar scope", "--exact")
                    browser("wait", f'[data-calendar-ticket="{idea_ticket["id"]}"]')
                    browser("screenshot", "/tmp/teleport-calendar.png")
                    dated = json.loads(tk("getTicket", "--data", json.dumps({"id": idea_ticket["id"]})))
                    assert dated["start"] == "2026-03-28" and dated["eta"] == "2026-03-30" and dated["status"] == "Todo"
                    browser("press", "Shift")
                    browser("press", "Shift")
                    browser("wait", "#finder-query")
                    browser("click", "#dismiss")
                except Exception:
                    profile.finish("failure")
                    profile.start("diagnostics")
                    try:
                        print(browser("eval", "({view,projectId,detailEpoch,loadGeneration,notice:document.querySelector('#notice')?.textContent,active:document.activeElement?.id,preferenceError,preferenceConflict,preferenceVersion:preferences?.version,sessionRevision:session?.revision,hidden:document.hidden,refreshing,graphInteracting,foreground:!!foregroundOpen()})"))
                        browser("screenshot", "/tmp/teleport-failure.png")
                    except Exception as diagnostic_error:
                        print("Smoke diagnostic unavailable: " + type(diagnostic_error).__name__, flush=True)
                    profile.finish("failure")
                    raise
                finally:
                    profile.start("browser-cleanup")
                    failed = sys.exc_info()[0] is not None
                    try:
                        browser("close")
                    except Exception:
                        profile.finish("failure")
                        if not failed:
                            raise
                    else:
                        profile.finish()
            tk("logout")
            tk("getSession", success=False)
            print("PASS: real binary init/serve/login/project/ticket/discovery/logout and exclusive storage ownership" + ("; browser acceptance verified" if args.browser else ""))
        finally:
            profile.start("server-cleanup")
            process.terminate()
            try:
                process.wait(timeout=1)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
            profile.finish()
        profile.start("reinitialization")
        assert "Initialization complete" in tk("init")
        profile.finish()

if __name__ == "__main__":
    from smoke_timeout import execute_worker, supervise, worker_directory
    from tool_history import run
    worker = worker_directory(__file__)
    if worker is not None:
        execute_worker(main, worker)
    else:
        from browser_setup import runtime_for_args
        run(lambda: supervise(__file__, python_executable=runtime_for_args(sys.argv[1:])))
