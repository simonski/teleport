"""Verify Board editing, scoped brakes, readiness and hover disclosure."""
import json


def verify_board_controls(browser, api, project, interactions_only=False):
    browser("click", '[data-view="Board"]')
    group = f'.board-group[data-board-project="{project["id"]}"][data-board-workflow="agent@1"]'
    browser("wait", group)
    browser("wait", "--fn", "[...document.querySelectorAll('[data-phase-workers]')].every(node=>/^0\\/1$/.test(node.textContent))")
    for action, key in ([] if interactions_only else [("board-brake", "agent#*"), ("lane-brake", "agent#Refine")]):
        selector = group + f' [data-board-action="{action}"]' + ('[data-board-phase="Refine"]' if action == 'lane-brake' else '')
        browser("scrollintoview", selector)
        browser("click", selector)
        browser("wait", "--fn", f"projects.find(p=>p.id==={project['id']})?.laneLocks?.[{json.dumps(key)}]===true")
        current = api("listProjects")["items"][0]
        assert current["handbrake"] and current["laneLocks"][key]
        browser("click", selector)
        browser("wait", "--fn", f"projects.find(p=>p.id==={project['id']})?.laneLocks?.[{json.dumps(key)}]===false")
    if not interactions_only:
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
    browser("eval", "{const first=document.querySelector('.board-group [data-ticket-card]');window.hoverNeighbour=first.nextElementSibling;window.hoverNeighbourTop=window.hoverNeighbour?.getBoundingClientRect().top;}")
    browser("hover", first)
    browser("wait", "--fn", "[...document.querySelectorAll('.ticket:hover .ticket-description')].some(node=>getComputedStyle(node).opacity==='1')")
    browser("eval", "{const text=document.querySelector('.ticket:hover .ticket-description');if(!text?.textContent||text.getBoundingClientRect().height<12)throw new Error('Hover failed to reveal description content');}")
    browser("eval", "{const text=document.querySelector('.ticket:hover .ticket-description'),style=getComputedStyle(text);if(style.position!=='absolute'||/height|margin|padding/.test(style.transitionProperty))throw new Error('Description animates layout');if(window.hoverNeighbour&&Math.abs(window.hoverNeighbour.getBoundingClientRect().top-window.hoverNeighbourTop)>.5)throw new Error('Hover moved a neighbouring ticket');}")
    browser("eval", "if(document.querySelector('.is-working'))throw new Error('Static simulation samples invented a running native worker');if(!document.querySelector('.readiness-label'))throw new Error('Readiness lacks a textual label')")
    browser("focus", first + ' .ticket-open')
    browser("screenshot", "/tmp/teleport-board-controls.png")
    draft = api("listTickets", {"projectId": project["id"], "kind": "task", "agent": True, "status": "Todo"})["items"][0]
    source = group + f' [data-ticket-card="{draft["id"]}"]'
    browser("scrollintoview", source)
    browser("eval", f"if(!document.querySelector({json.dumps(source)}).draggable)throw new Error('Board source is not draggable')")
    browser("drag", source, group + ' [data-phase-id="Refine"] .lane-body')
    browser("wait", "--fn", f"tickets.find(ticket=>ticket.id==={draft['id']})?.status==='Refine'")
    moved = api("getTicket", {"id": draft["id"]})
    assert moved["workflow"] == draft["workflow"] and moved["agent"] and not moved["active"]
    browser("eval", "if(boardTicketDrag||document.querySelector('.board-drag-in-progress'))throw new Error('Drop retained drag ownership')")
    print("PASS: Board controls, fixed-layout compositor hover reveal, stable neighbouring tickets and real Todo-to-Refine drag through the pinned queue action", flush=True)
