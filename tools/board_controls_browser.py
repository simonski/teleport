"""Verify Board editing, scoped brakes, readiness and hover disclosure."""
import json


def verify_board_controls(browser, api, project):
    browser("click", '[data-view="Board"]')
    group = f'.board-group[data-board-project="{project["id"]}"][data-board-workflow="agent@1"]'
    browser("wait", group)
    browser("wait", "--fn", "[...document.querySelectorAll('[data-phase-workers]')].every(node=>/^0\\/1$/.test(node.textContent))")
    for action, key in [("board-brake", "agent#*"), ("lane-brake", "agent#Refine")]:
        selector = group + f' [data-board-action="{action}"]' + ('[data-board-phase="Refine"]' if action == 'lane-brake' else '')
        browser("scrollintoview", selector)
        browser("click", selector)
        browser("wait", "--fn", f"projects.find(p=>p.id==={project['id']})?.laneLocks?.[{json.dumps(key)}]===true")
        current = api("listProjects")["items"][0]
        assert current["handbrake"] and current["laneLocks"][key]
        browser("click", selector)
        browser("wait", "--fn", f"projects.find(p=>p.id==={project['id']})?.laneLocks?.[{json.dumps(key)}]===false")
    edit = group + ' [data-board-action="edit"][data-board-phase="Ready"]'
    browser("scrollintoview", edit)
    browser("click", edit)
    browser("wait", "#flow-draft")
    browser("scrollintoview", "#flow-draft")
    browser("click", "#flow-draft")
    browser("wait", "--fn", "flow&&!flow.published")
    selector = '[data-phase="2"] [data-ready-admission] select'
    browser("scrollintoview", '[data-phase="2"] [data-ready-admission] .choice-button')
    browser("select", selector, "human")
    browser("scrollintoview", "#flow-save")
    browser("click", "#flow-save")
    browser("wait", "--fn", "!flowDirty&&!workflowMutation")
    assert next(phase for phase in api("getWorkflow", {"workflowKey": "agent@draft"})["phases"] if phase["role"] == "Ready")["requireReadyReview"]
    browser("click", '[data-view="Board"]')
    browser("wait", group)
    first = group + ' [data-ticket-card]'
    browser("scrollintoview", first)
    browser("hover", first)
    browser("wait", "--fn", "[...document.querySelectorAll('.ticket:hover .ticket-description')].some(node=>getComputedStyle(node).opacity==='1')")
    browser("eval", "{const text=document.querySelector('.ticket:hover .ticket-description');if(!text?.textContent||text.getBoundingClientRect().height<12)throw new Error('Hover failed to reveal description content');}")
    browser("eval", "if(document.querySelector('.is-working'))throw new Error('Static simulation samples invented a running native worker');if(!document.querySelector('.readiness-label'))throw new Error('Readiness lacks a textual label')")
    browser("focus", first + ' .ticket-open')
    browser("screenshot", "/tmp/teleport-board-controls.png")
    print("PASS: Board/lane handbrakes preserve project scope, active/configured counts, workflow edit shortcut, persisted Ready review option and delayed hover description", flush=True)
