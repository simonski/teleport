"""Real-browser acceptance for project summaries, relationship lists and assembly."""
import json
import time
from backlog_browser import verify_backlog


def verify_overview(browser, tk, project, stage=lambda name: None):
    def call(operation, data):
        return json.loads(tk(operation, "--data", json.dumps(data)))

    browser("set", "viewport", "1600", "1000")
    goal = call("createTicket", {"projectId": project["id"], "kind": "goal", "title": "Launch the project", "agent": False, "requestId": "overview-goal"})
    epic = call("createTicket", {"projectId": project["id"], "kind": "epic", "parentId": goal["id"], "title": "A clearer ticket workspace", "agent": False, "requestId": "overview-epic"})
    stories = []
    for index, title in enumerate(("Ship the foundations", "Connect epic and stories", "Review project progress", "Next delivery")):
        ticket = call("createTicket", {"projectId": project["id"], "parentId": epic["id"], "title": title, "agent": False, "requestId": "overview-" + str(index)})
        if index < 3:
            ticket = call("transitionTicket", {"id": ticket["id"], "expectedVersion": ticket["version"], "target": "Doing"})
        if index == 0:
            ticket = call("transitionTicket", {"id": ticket["id"], "expectedVersion": ticket["version"], "target": "Done", "confirm": True})
        stories.append(ticket)
    browser("eval", "workspace().catch(error=>{if(!error.staleInteraction)throw error;})")
    browser("wait", "--fn", "homeSummary?.completed>=1&&homeSummary?.remaining>=3")
    browser("eval", "if(document.querySelector('#new')||document.querySelectorAll('.summary-pie svg[role=img]').length!==2||!document.querySelector('.summary-metrics').textContent.includes('Not metered'))throw new Error('Summary metrics or shortcut-only creation missing')")
    browser("screenshot", "/tmp/teleport-project-home.png")
    browser("select", "#home-period", "30")
    browser("wait", "--fn", "homeSummary?.days===30")
    browser("eval", "document.activeElement?.blur()")
    browser("press", "n"); browser("wait", '#editor [name="title"]'); browser("press", "Escape")
    stage("overview-backlog")
    verify_backlog(browser,tk,project,goal,epic,stories,stage)
    assert call("getTicket", {"id": stories[1]["id"]})["version"] == stories[1]["version"]
    stage("overview-attention")
    browser("click", "#account"); browser("eval", "document.querySelector('#account-attention').focus()")
    browser("press", "ArrowRight"); browser("press", "Escape")
    browser("eval", "if(document.querySelector('#account-menu').hidden||!document.querySelector('#attention-submenu').hidden)throw new Error('Empty Attention submenu lost its parent')")
    browser("press", "Escape")
    agent = call("createTicket", {"projectId": project["id"], "title": "Review this proposed scope", "requestId": "overview-attention"})
    settings = json.loads(tk("getSession"))["settings"]
    call("setExecutionMode", {"mode": "demo", "expectedVersion": settings["version"]})
    call("requestRefinement", {"id": agent["id"], "expectedVersion": agent["version"]})
    deadline = time.monotonic() + 5
    while time.monotonic() < deadline:
        agent = call("getTicket", {"id": agent["id"]})
        if agent.get("attention"):
            break
        time.sleep(.05)
    assert agent.get("attention")
    settings = json.loads(tk("getSession"))["settings"]
    call("setExecutionMode", {"mode": "off", "expectedVersion": settings["version"]})
    browser("eval", "workspace().catch(error=>{if(!error.staleInteraction)throw error;})")
    browser("wait", "--fn", "tickets.some(ticket=>ticket.id===" + str(agent["id"]) + "&&ticket.attention)")
    browser("click", "#account")
    browser("eval", "document.querySelector('#account-attention').focus()")
    browser("press", "ArrowRight")
    browser("wait", "--fn", "!document.querySelector('#attention-submenu').hidden")
    browser("screenshot", "/tmp/teleport-profile-attention.png")
    browser("press", "Escape")
    browser("eval", "if(!document.querySelector('#attention-submenu').hidden||document.querySelector('#account-menu').hidden||document.activeElement.id!=='account-attention')throw new Error('Submenu Escape lost its parent')")
    browser("press", "ArrowRight")
    browser("click", '[data-attention-ticket="' + str(agent["id"]) + '"]')
    browser("wait", '#editor [name="title"]'); browser("press", "Escape")
    assert call("getTicket", {"id": agent["id"]})["attention"]
    stage("overview-assembly")
    draft = call("copyWorkflow", {"sourceKey": "human@1", "projectId": project["id"], "title": "Assemble a delivery", "requestId": "overview-workflow"})
    key = draft["id"] + "@draft"
    browser("click", '#nav-views [data-view="Workflows"]')
    browser("wait", "#flow-select"); browser("click", "#workflow-view-map")
    browser("wait", "--fn", "!!workflowMapRuntime")
    browser("eval", "if([...document.querySelectorAll('[data-inventory-step]')].some(button=>!button.disabled))throw new Error('Published inventory is editable')")
    browser("select", "#flow-select", key)
    browser("click", "#workflow-view-map")
    browser("wait", "--fn", "!!workflowMapRuntime&&!!workflowInventoryDispose")
    before = json.loads(browser("eval", "flow.phases.length"))
    browser("eval", "window.inventoryBeforeIds=flow.phases.map(phase=>phase.id)")
    source = json.loads(browser("eval", "(()=>{const r=document.querySelector('[data-inventory-step=\"3\"]').getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2};})()"))
    target = json.loads(browser("eval", "(()=>{const r=document.querySelector('#workflow-map-stage').getBoundingClientRect();return {x:r.x+r.width*.72,y:r.y+r.height*.78};})()"))
    browser("mouse", "move", str(source["x"]), str(source["y"])); browser("mouse", "down", "left")
    browser("mouse", "move", str(target["x"]), str(target["y"])); browser("press", "Escape"); browser("mouse", "up", "left")
    browser("eval", "if(flowDirty||document.querySelector('.inventory-ghost')||flowDrag)throw new Error('Cancelled inventory placement changed the draft')")
    browser("mouse", "move", str(source["x"]), str(source["y"])); browser("mouse", "down", "left")
    browser("mouse", "move", str(target["x"]), str(target["y"])); browser("mouse", "up", "left")
    browser("wait", "--fn", "flow.phases.length===" + str(before + 1) + "&&!!workflowMapRuntime")
    added = json.loads(browser("eval", "flow.phases.find(phase=>!window.inventoryBeforeIds.includes(phase.id)).id"))
    browser("eval", "workflowMapRuntime.scene.select(" + json.dumps(added) + ")")
    browser("eval", "document.querySelector('[data-inventory-step=\"3\"]').focus()")
    browser("press", "Enter")
    browser("wait", "--fn", "flow.phases.length===" + str(before + 2) + "&&!!workflowMapRuntime")
    browser("eval", "if(!flow.rules.some(rule=>rule.source===" + json.dumps(added) + "&&rule.outcome==='complete')||!workflowTrace)throw new Error('Nearby inventory steps did not connect')")
    browser("click", "#workflow-map-fit")
    browser("screenshot", "/tmp/teleport-workflow-step-inventory.png")
    browser("click", "#flow-save")
    browser("wait", "--fn", "!flowDirty&&!workflowMutation&&!!workflowMapRuntime")
    persisted = call("getWorkflow", {"workflowKey": key})
    assert not persisted["published"] and len(persisted["phases"]) == before + 2
    assert next(item for item in json.loads(tk("listProjects"))["items"] if item["id"] == project["id"])["humanWorkflow"] == "human@1"
    browser("set", "viewport", "390", "844")
    browser("screenshot", "/tmp/teleport-workflow-inventory-mobile.png")
    browser("click", '#nav-views [data-view="Home"]')
    browser("wait", "--fn", "!!homeSummary")
    browser("eval", "if(document.documentElement.scrollWidth>innerWidth)throw new Error('Home overflows mobile')")
    browser("screenshot", "/tmp/teleport-project-home-mobile.png")
    print("PASS: scoped Home pies/metrics, shortcut-only N, epic/story workflow list, nested Attention keyboard/routes, inventory pointer/keyboard assembly and safe draft persistence")
