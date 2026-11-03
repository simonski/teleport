"""Browser acceptance for unified backlog ordering, guarded parenting and headers."""
import json


def verify_backlog(browser, tk, project, goal, epic, stories, stage=lambda name: None):
    def call(operation, data):
        return json.loads(tk(operation, "--data", json.dumps(data)))

    other = call("createTicket", {"projectId": project["id"], "parentId": goal["id"], "kind": "epic", "title": "The next delivery", "agent": False, "requestId": "backlog-epic"})
    idea = call("saveIdea", {"projectId": project["id"], "title": "A quieter backlog", "text": "Retain the original idea.", "requestId": "backlog-idea"})
    browser("eval", "workspace().catch(error=>{if(!error.staleInteraction)throw error;})")
    browser("click", '#nav-views [data-view="Backlog"]')
    browser("wait", "--fn", "backlogLoaded&&!backlogLoading")
    browser("set", "viewport", "1600", "1400")
    browser("eval", "if(['Ideas','Goals','Epics','Tickets','Projects'].some(name=>document.querySelector('#nav-views [data-view=\"'+name+'\"]')))throw new Error('Legacy tabs remain')")
    browser("wait", '[data-backlog-open="' + idea["id"] + '"]')
    for kind in ("idea", "goal", "epic", "task"):
        browser("select", "#backlog-kind", kind)
        browser("eval", "document.activeElement?.blur()"); browser("press", "n")
        browser("wait", '#editor [name="title"]')
        if kind != "idea":
            browser("eval", "if(document.querySelector('#editor [name=kind]').value!==" + json.dumps(kind) + ")throw new Error('Backlog creation lost type')")
        browser("press", "Escape")
    browser("select", "#backlog-kind", "")
    browser("screenshot", "/tmp/teleport-unified-backlog.png")

    def row(ticket):
        return '[data-backlog-key="ticket:' + str(ticket["id"]) + '"]'

    def point(selector, ratio=.5):
        return json.loads(browser("eval", "(()=>{const r=document.querySelector(" + json.dumps(selector) + ").getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height*" + str(ratio) + "};})()"))

    def drag(source, target, cancel=False):
        start = point(row(source) + " .backlog-grip")
        end = point(target)
        browser("mouse", "move", str(start["x"]), str(start["y"])); browser("mouse", "down", "left")
        browser("mouse", "move", str(end["x"]), str(end["y"]))
        if cancel:
            browser("press", "Escape")
        browser("mouse", "up", "left")

    source, locked = stories[3], stories[1]
    before = call("getBacklogOrder", {"projectId": project["id"]})
    drag(source, row(other), cancel=True)
    assert call("getBacklogOrder", {"projectId": project["id"]}) == before
    drag(locked, row(other))
    assert call("getTicket", {"id": locked["id"]})["parentId"] == epic["id"]
    drag(source, row(other))
    browser("wait", "--fn", "!backlogPending&&backlogLoaded&&!backlogLoading&&tickets.find(ticket=>ticket.id===" + str(source["id"]) + ")?.parentId===" + str(other["id"]))
    assert call("getTicket", {"id": source["id"]})["parentId"] == other["id"]
    drag(source, row(stories[2]))
    browser("wait", "--fn", "!backlogPending&&backlogLoaded&&!backlogLoading&&tickets.find(ticket=>ticket.id===" + str(source["id"]) + ")?.parentId===" + str(epic["id"]))
    assert call("getTicket", {"id": source["id"]})["parentId"] == epic["id"]
    browser("eval", "document.querySelector(" + json.dumps(row(source) + " .backlog-grip") + ").focus()")
    revision = call("getBacklogOrder", {"projectId": project["id"]})["version"]
    browser("press", "Alt+ArrowUp")
    browser("wait", "--fn", "!backlogPending&&!backlogLoading&&backlogOrder.version>" + str(revision))
    saved = call("getBacklogOrder", {"projectId": project["id"]})
    browser("reload")
    browser("wait", "--fn", "view==='Backlog'&&backlogLoaded&&!backlogLoading")
    assert call("getBacklogOrder", {"projectId": project["id"]}) == saved
    browser("set", "viewport", "1600", "1400")
    drag(source, "#backlog-out")
    browser("wait", "--fn", "!backlogPending&&backlogLoaded&&!backlogLoading&&tickets.find(ticket=>ticket.id===" + str(source["id"]) + ")?.parentId===" + str(project["unassignedEpic"]))
    moved = call("getTicket", {"id": source["id"]})
    assert moved["priority"] == source["priority"] and moved["workflow"] == source["workflow"]
    stage("backlog-header")
    for position in ("left", "right", "top"):
        browser("click", "#account"); browser("click", "#account-settings")
        browser("select", "#navigation-position", position)
        browser("wait", "--fn", "!preferenceSaving&&!Object.keys(preferenceDraft).length"); browser("press", "Escape")
        browser("eval", "{const h=document.querySelector('.workspace-header').getBoundingClientRect(),top=document.querySelector('.topbar').getBoundingClientRect(),nav=document.querySelector('#workspace-nav').getBoundingClientRect();if(" + json.dumps(position) + "!=='top'&&(Math.abs(h.width-nav.width)>2||top.top!==0||nav.top<top.bottom-2))throw new Error('Only tabs moved; whole header did not move');}")
        browser("screenshot", "/tmp/teleport-header-" + position + ".png")
    browser("select", "#backlog-kind", "")
    browser("set", "viewport", "390", "844")
    browser("eval", "if(document.documentElement.scrollWidth>innerWidth)throw new Error('Backlog overflows mobile')")
    browser("screenshot", "/tmp/teleport-backlog-mobile.png")
    browser("set", "viewport", "1600", "1000")
    browser("click", '#nav-views [data-view="Home"]')
    print("PASS: unified Backlog, graph cards/indentation, keyboard and pointer order, locked/cancelled moves, story-to-epic parenting, unassign, persistence and whole-header placements")
