"""Focused browser acceptance for the bundled Three.js graph renderer."""
import json
import math


def verify_camera_retained(browser):
    before = json.loads(json.loads(browser("eval", "window.__graphCamera")))
    after = json.loads(browser("eval", "graphRuntime.scene.snapshot()"))
    assert before["selected"] == after["selected"], "Graph refresh changed selection"
    for field in ("camera", "target"):
        assert len(before[field]) == len(after[field]) == 3
        assert all(math.isclose(a, b, rel_tol=1e-12, abs_tol=1e-9)
                   for a, b in zip(before[field], after[field])), "Graph refresh moved " + field


def verify_graph(browser, tk, project_id, ticket):
    def call(operation, data):
        return json.loads(tk(operation, "--data", json.dumps(data)))

    browser("select", "#project", str(project_id))
    browser("find", "role", "button", "click", "--name", "Graph", "--exact")
    browser("click", "#graph-3d")
    browser("wait", "--fn", "!!graphRuntime?.scene&&!!document.querySelector('#graph-stage canvas')")
    browser("click", f'[data-node="ticket:{ticket["id"]}"]')
    browser("fill", '[data-coordinate="x"]', "125")
    browser("click", "#graph-apply-position")
    browser("click", "#graph-save")
    browser("wait", "--fn", "document.querySelector('#graph-state')?.innerText.includes('saved')")
    graph = call("getGraph", {"projectId": project_id})
    assert graph["layout"]["positions"][f'ticket:{ticket["id"]}']["x"] == 125
    assert call("getTicket", {"id": ticket["id"]})["version"] == ticket["version"]
    browser("eval", "window.__graphCamera=JSON.stringify(graphRuntime.scene.snapshot())")
    changed = call("updateTicket", {"id": ticket["id"], "expectedVersion": ticket["version"], "priority": "high"})
    browser("wait", "--fn", f'tickets.some(t=>t.id==={ticket["id"]}&&t.version==={changed["version"]})&&!!graphRuntime')
    verify_camera_retained(browser)
    browser("screenshot", "/tmp/teleport-dependency-graph.png")
    browser("find", "role", "button", "click", "--name", "Settings", "--exact")
    browser("select", "#appearance-style", "instrument")
    browser("select", "#appearance-mode", "dark")
    browser("wait", "--fn", "document.documentElement.dataset.mode==='dark'&&!preferenceSaving&&!Object.keys(preferenceDraft).length")
    browser("press", "Escape")
    browser("screenshot", "/tmp/teleport-dependency-graph-dark.png")
    browser("set", "viewport", "390", "844")
    browser("eval", "if(document.documentElement.scrollWidth>innerWidth)throw new Error('Graph overflows mobile width')")
    browser("screenshot", "/tmp/teleport-dependency-graph-mobile.png")
    browser("set", "viewport", "1280", "800")
    browser("reload")
    browser("wait", "#welcome-status")
    browser("press", "Escape")
    browser("click", "#graph-3d")
    browser("wait", "--fn", "!!graphRuntime?.scene&&!!document.querySelector('#graph-stage canvas')")
    graph = call("getGraph", {"projectId": project_id})
    assert graph["layout"]["positions"][f'ticket:{ticket["id"]}']["x"] == 125
    browser("find", "role", "button", "click", "--name", "Home", "--exact")
    browser("eval", "window.__nativeContext=HTMLCanvasElement.prototype.getContext;HTMLCanvasElement.prototype.getContext=function(kind,...args){return kind.startsWith('webgl')?null:window.__nativeContext.call(this,kind,...args)}")
    browser("find", "role", "button", "click", "--name", "Graph", "--exact")
    browser("wait", "--fn", "document.querySelector('#graph-stage')?.innerText.includes('WebGL is unavailable')")
    browser("click", f'[data-node="ticket:{ticket["id"]}"]')
    browser("wait", "#graph-apply-position")
    browser("screenshot", "/tmp/teleport-dependency-graph-fallback.png")
    browser("eval", "HTMLCanvasElement.prototype.getContext=window.__nativeContext")
    print("PASS: Three.js WebGL graph, saved layout/reload, live camera retention, dark/mobile rendering and usable non-WebGL fallback")
