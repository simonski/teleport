"""Verify lane metadata through the real workflow editor and server API."""
import json


def verify_workflow_lanes(browser, tk, project):
    draft = json.loads(tk("copyWorkflow", "--data", json.dumps({"sourceKey": "agent@1", "projectId": project["id"], "title": "Lane configuration", "requestId": "lane-configuration"})))
    browser("click", '#nav-views [data-view="Home"]')
    browser("click", '#nav-views [data-view="Workflows"]')
    browser("wait", "--fn", "!workflowLoading&&!!document.querySelector('#flow-select')")
    browser("select", "#flow-select", draft["id"] + "@draft")
    browser("wait", "--fn", "!workflowMutation&&flow.family==='agent'")
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
