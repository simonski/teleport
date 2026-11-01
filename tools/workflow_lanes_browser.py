"""Verify lane metadata through the real workflow editor and server API."""
import json


def verify_workflow_lanes(browser, tk, project):
    draft = json.loads(tk("copyWorkflow", "--data", json.dumps({"sourceKey": "agent@1", "projectId": project["id"], "title": "Lane configuration", "requestId": "lane-configuration"})))
    browser("click", '#nav-views [data-view="Home"]')
    browser("click", '#nav-views [data-view="Workflows"]')
    browser("wait", "--fn", "!workflowLoading&&!!document.querySelector('#flow-select')")
    browser("select", "#flow-select", draft["id"] + "@draft")
    browser("wait", "--fn", "!workflowMutation&&flow.family==='agent'")
    verify_lane_order(browser, tk, draft)
    row = '[data-phase="1"]'
    browser("eval", "if(document.querySelector('[data-phase=\"0\"] [data-worker-field]').getBoundingClientRect().height)throw new Error('Manual lane exposes workers')")
    description = 'Clarify <scope> & preserve meaning.'
    browser("fill", row + ' [data-field="description"]', description)
    browser("fill", row + ' [data-field="workers"]', "1.5")
    browser("click", "#flow-save")
    browser("wait", "--fn", "document.querySelector('#flow-message').textContent.includes('whole number')")
    unchanged = json.loads(tk("getWorkflow", "--data", json.dumps({"workflowKey": draft["id"] + "@draft"})))
    assert unchanged["draftRevision"] == draft["draftRevision"]
    browser("fill", row + ' [data-field="workers"]', "0")
    browser("click", "#flow-save")
    browser("wait", "--fn", "!workflowMutation&&document.querySelector('#flow-message').textContent.includes('Draft saved')")
    saved = json.loads(tk("getWorkflow", "--data", json.dumps({"workflowKey": draft["id"] + "@draft"})))
    assert saved["phases"][1]["workers"] == 0 and saved["phases"][1]["description"] == description
    browser("click", "#flow-discard")
    browser("wait", "--fn", "!workflowLoading&&document.querySelector('[data-phase=\"1\"] [data-field=workers]').value==='0'")
    browser("fill", row + ' [data-field="workers"]', "2")
    browser("click", row + ' [data-field="agentic"]')
    browser("eval", "if('workers' in flow.phases[1]||!document.querySelector('[data-phase=\"1\"] [data-worker-field]').hidden)throw new Error('Manual toggle retained an agent quota')")
    browser("click", row + ' [data-field="agentic"]')
    browser("fill", row + ' [data-field="workers"]', "")
    browser("click", "#flow-save")
    browser("wait", "--fn", "!workflowMutation&&document.querySelector('#flow-message').textContent.includes('Draft saved')")
    saved = json.loads(tk("getWorkflow", "--data", json.dumps({"workflowKey": draft["id"] + "@draft"})))
    assert "workers" not in saved["phases"][1]
    browser("fill", row + ' [data-field="workers"]', "2")
    browser("click", "#flow-save")
    browser("wait", "--fn", "!workflowMutation&&document.querySelector('#flow-message').textContent.includes('Draft saved')")
    browser("click", "#flow-publish")
    browser("wait", "--fn", "!workflowPublicationPending&&flow.published")
    published = json.loads(tk("getWorkflow", "--data", json.dumps({"workflowKey": draft["id"] + "@1"})))
    assert published["phases"][1]["workers"] == 2 and published["phases"][1]["description"] == description
    browser("eval", "if(!document.querySelector('[data-phase=\"1\"]').disabled)throw new Error('Published fields are editable')")
    browser("eval", "document.querySelector('[data-phase=\"1\"]').scrollIntoView({block:'start'})")
    browser("screenshot", "/tmp/teleport-workflow-lane.png")
    browser("set", "viewport", "390", "844")
    browser("eval", "if(document.documentElement.scrollWidth>innerWidth)throw new Error('Lane fields overflow mobile')")
    browser("screenshot", "/tmp/teleport-workflow-lane-mobile.png")
    browser("set", "viewport", "1280", "800")


def verify_lane_order(browser, tk, draft):
    def order():
        return json.loads(browser("eval", "flow.phases.map(phase=>phase.id)"))

    def point(selector, fraction=0.5):
        return json.loads(browser("eval", "(()=>{const r=document.querySelector(" + json.dumps(selector) + ").getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height*" + str(fraction) + "};})()"))

    def move(point):
        browser("mouse", "move", str(round(point["x"])), str(round(point["y"])))

    initial = order()
    browser("click", '[data-lane-down="0"]')
    assert order()[:2] == initial[1::-1]
    browser("press", "Alt+ArrowUp")
    assert order() == initial
    def start_drag():
        browser("eval", "document.querySelector('[data-lane-grip=\"0\"]').scrollIntoView({block:'center'})")
        origin = point('[data-lane-grip="0"]')
        move(origin); browser("mouse", "down", "left")
        move({"x": origin["x"], "y": origin["y"] + 10})
        browser("eval", "document.querySelector('[data-phase=\"1\"]').scrollIntoView({block:'center'})")
        move(point('[data-phase="1"]', 0.8))

    start_drag()
    browser("wait", ".lane-drag-ghost")
    browser("eval", "if(flow.phases[0].role!=='Todo')throw new Error('Pointer preview changed the draft before drop')")
    browser("press", "Escape"); browser("mouse", "up", "left")
    assert order() == initial
    browser("eval", "if(workflowLaneDrag||flowDrag||document.querySelector('.lane-drag-ghost'))throw new Error('Cancelled drag retained input ownership')")
    start_drag(); browser("mouse", "up", "left")
    assert order()[:2] == initial[1::-1]
    browser("click", "#flow-save")
    browser("wait", "--fn", "!workflowMutation&&document.querySelector('#flow-message').textContent.includes('Draft saved')")
    saved = json.loads(tk("getWorkflow", "--data", json.dumps({"workflowKey": draft["id"] + "@draft"})))
    assert [phase["id"] for phase in saved["phases"]] == order()
    assert saved["rules"] == draft["rules"] and saved.get("layout") == draft.get("layout")
    browser("click", "#flow-discard")
    browser("wait", "--fn", "!workflowLoading&&flow.phases[0].role==='Refine'")
    # Restore the original order for the field-specific assertions below.
    browser("click", '[data-lane-up="1"]')
    browser("click", "#flow-save")
    browser("wait", "--fn", "!workflowMutation&&document.querySelector('#flow-message').textContent.includes('Draft saved')")
    draft["draftRevision"] = json.loads(tk("getWorkflow", "--data", json.dumps({"workflowKey": draft["id"] + "@draft"})))["draftRevision"]
    assert order() == initial
