"""Real-browser checks for authorized project aggregates and metadata editing."""
import json


def verify_project_directory(browser, tk, selected_project):
    def call(operation, data=None):
        return json.loads(tk(operation, "--data", json.dumps(data or {})))

    projects = call("listProjects")["items"]
    other = next(project for project in projects if project["id"] != selected_project)
    call("createTicket", {"projectId": other["id"], "title": "Directory outside selected scope", "agent": False, "requestId": "directory-other"})
    browser("click", '#nav-views [data-view="Projects"]')
    browser("click", '[data-project-presentation="list"]')
    browser("wait", "--fn", "!!projectDirectory&&!document.querySelector('#project-directory-status').textContent")
    rows = call("listProjectSummaries")["items"]
    for row in rows:
        expected = len([ticket for ticket in call("listTickets", {"projectId": row["id"]})["items"] if not ticket.get("protected")])
        assert row["tickets"] == expected
        prefix = f'[data-project-row="{row["id"]}"]'
        assert json.loads(browser("eval", "document.querySelector(" + json.dumps(prefix + ' [data-metric="tickets"]') + ").textContent")) == str(expected)
        assert json.loads(browser("eval", "document.querySelector(" + json.dumps(prefix + ' [data-metric="tokens"]') + ").textContent")) == "Unavailable"
    assert json.loads(browser("eval", "projectId")) == selected_project
    browser("click", f'[data-project-edit="{selected_project}"]')
    browser("wait", '#editor [name="description"]')
    description = "Flights <b>kept literal</b>"
    browser("fill", '#editor [name="description"]', description)
    browser("click", '#editor [type="submit"]')
    browser("wait", "--fn", "!document.querySelector('tk-dialog[open]')&&!document.querySelector('#project-directory-status')?.textContent&&!!projectDirectory")
    prefix = f'[data-project-row="{selected_project}"]'
    browser("wait", "--fn", "document.querySelector(" + json.dumps(prefix + ' .project-description') + ")?.textContent===" + json.dumps(description))
    browser("eval", "if(document.querySelector('.project-description b'))throw new Error('Project description was interpreted as HTML')")
    saved = next(project for project in call("listProjects")["items"] if project["id"] == selected_project)
    assert saved["description"] == description
    browser("screenshot", "/tmp/teleport-project-list.png")
    browser("set", "viewport", "390", "844")
    browser("eval", "if(document.documentElement.scrollWidth>innerWidth)throw new Error('Project list overflows the page instead of its scroll region')")
    browser("screenshot", "/tmp/teleport-project-list-mobile.png")
    browser("set", "viewport", "1280", "800")
    browser("click", '[data-project-presentation="cards"]')
    browser("wait", f'.project-card[data-project="{other["id"]}"]')
    browser("screenshot", "/tmp/teleport-project-cards.png")
    browser("click", f'.project-card[data-project="{other["id"]}"]')
    browser("wait", "--fn", f'projectId==={other["id"]}&&view==="Board"')
    browser("select", "#project", str(selected_project))
    browser("click", '#nav-views [data-view="Home"]')
    print("PASS: server-scoped project Card/List totals beyond selected project, honest missing usage, escaped versioned descriptions, mobile table scrolling and exact project navigation")
