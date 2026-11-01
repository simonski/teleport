"""Shared real-browser assertions for persisted navigation and project creation."""
import json


def verify_navigation(browser, tk, project_id):
    def saved():
        browser("wait", "--fn", "!preferenceSaving&&!Object.keys(preferenceDraft).length")

    def prefs():
        return json.loads(tk("getViewPreferences"))

    def visible():
        return json.loads(browser("eval", "[...document.querySelectorAll('#nav-views [data-view]')].map(node=>node.dataset.view)"))

    def rect(name, fraction=0.5):
        selector = '#nav-views [data-view=' + json.dumps(name) + ']'
        return json.loads(browser("eval", "(()=>{const r=document.querySelector(" + json.dumps(selector) + ").getBoundingClientRect();return {x:r.x+r.width*" + str(fraction) + ",y:r.y+r.height/2};})()"))

    def point(kind, value):
        browser("mouse", kind, str(round(value["x"])), str(round(value["y"])))

    expected = ["Home", "Ideas", "Goals", "Epics", "Board", "Todos", "Calendar", "Graph", "Cost", "Projects"]
    appearance_before = prefs()
    assert visible() == expected
    browser("eval", "document.querySelector('#nav-views [data-view=Ideas]').focus()")
    browser("press", "Alt+Shift+ArrowLeft")
    saved()
    assert visible()[0] == "Ideas" and prefs()["navigationOrder"][0] == "Ideas"
    revision = json.loads(tk("getSession"))["revision"]
    browser("wait", "--fn", "session.revision>=" + str(revision))
    browser("eval", "if(document.activeElement?.dataset.view!=='Ideas')throw new Error('Preference refresh lost keyboard tab focus')")
    browser("eval", "if(document.querySelector('[aria-current=page]')?.dataset.view!=='Home')throw new Error('Reorder navigated away')")

    origin, destination = rect("Ideas"), rect("Graph", 0.8)
    point("move", origin); browser("mouse", "down", "left"); point("move", destination); browser("mouse", "up", "left")
    saved()
    order = prefs()["navigationOrder"]
    assert order.index("Ideas") == order.index("Graph") + 1, order
    browser("eval", "if(document.querySelector('[aria-current=page]')?.dataset.view!=='Home'||document.querySelector('.nav-drag-ghost'))throw new Error('Drop clicked a view or retained a ghost')")

    before_cancel = prefs()
    origin, destination = rect("Ideas"), rect("Home", 0.1)
    point("move", origin); browser("mouse", "down", "left"); point("move", destination)
    browser("press", "Escape"); browser("mouse", "up", "left")
    saved()
    assert prefs() == before_cancel, "Cancelled drag changed persistent preferences"

    browser("find", "role", "button", "click", "--name", "Settings", "--exact")
    browser("eval", "window.navigationMain=document.querySelector('main')")
    browser("select", "#navigation-position", "left"); saved()
    browser("eval", "if(document.documentElement.dataset.navigation!=='left'||window.navigationMain!==document.querySelector('main'))throw new Error('Layout replaced content')")
    browser("press", "Escape")
    assert prefs()["navigationPosition"] == "left"
    revision = json.loads(tk("getSession"))["revision"]
    browser("wait", "--fn", "session.revision>=" + str(revision))
    browser("eval", "if(document.activeElement!==document.querySelector('#account'))throw new Error('Background preference refresh lost popup return focus')")
    browser("screenshot", "/tmp/teleport-navigation-left.png")
    browser("click", "#nav-collapse"); saved()
    assert prefs()["navigationCollapsed"]
    browser("screenshot", "/tmp/teleport-navigation-collapsed.png")

    browser("find", "role", "button", "click", "--name", "Settings", "--exact")
    browser("select", "#navigation-position", "right"); saved(); browser("press", "Escape")
    browser("click", "#nav-collapse"); saved()
    assert prefs()["navigationPosition"] == "right" and not prefs()["navigationCollapsed"]
    browser("screenshot", "/tmp/teleport-navigation-right.png")
    browser("find", "role", "button", "click", "--name", "Settings", "--exact")
    browser("select", "#appearance-style", "instrument"); browser("select", "#appearance-mode", "dark"); saved(); browser("press", "Escape")
    browser("eval", "if(Math.abs(document.querySelector('.topbar').getBoundingClientRect().height-59)>1||Math.abs(document.querySelector('#workspace-nav').getBoundingClientRect().top-59)>1)throw new Error('Theme changed compact header/sidebar geometry')")
    browser("screenshot", "/tmp/teleport-navigation-dark.png")
    browser("click", "#nav-collapse"); saved()
    browser("reload")
    browser("wait", "--fn", "document.documentElement.dataset.navigation==='right'&&document.documentElement.dataset.navigationCollapsed==='true'&&document.querySelector('#nav-views')")
    assert prefs()["navigationOrder"] == order
    browser("select", "#project", str(project_id))
    browser("set", "viewport", "390", "844")
    browser("click", "#nav-collapse"); saved()
    browser("eval", "if(document.documentElement.scrollWidth>innerWidth)throw new Error('Side navigation overflows mobile width')")
    browser("eval", "if(Math.abs(document.querySelector('.topbar').getBoundingClientRect().height-48)>1||Math.abs(document.querySelector('#workspace-nav').getBoundingClientRect().top-48)>1)throw new Error('Mobile theme header overlaps sidebar')")
    browser("screenshot", "/tmp/teleport-navigation-mobile.png")
    browser("click", "#nav-collapse"); saved()
    browser("set", "viewport", "1280", "800")

    browser("find", "role", "button", "click", "--name", "Settings", "--exact")
    browser("select", "#navigation-position", "top"); saved(); browser("press", "Escape")
    browser("eval", "if(getComputedStyle(document.querySelector('#nav-views .tab-label')).display==='none')throw new Error('Collapsed sidebar hid top labels')")
    browser("find", "role", "button", "click", "--name", "Settings", "--exact")
    browser("select", "#appearance-style", appearance_before["style"]); browser("select", "#appearance-mode", appearance_before["mode"])
    browser("click", "#navigation-reset"); saved(); browser("press", "Escape")
    assert visible() == expected and prefs()["navigationPosition"] == "top" and not prefs()["navigationCollapsed"]

    projects_before = json.loads(tk("listProjects"))["items"]
    browser("click", "#project + .choice-button")
    browser("eval", "if(document.querySelector('.choice-group-title')?.textContent!=='New project')throw new Error('Missing creation section')")
    browser("click", '.choice-list [data-value="new-project"]')
    browser("wait", '#editor [name="name"]')
    browser("eval", "if(document.activeElement!==document.querySelector('#editor [name=name]'))throw new Error('New project text entry not focused')")
    browser("press", "Escape")
    assert json.loads(tk("listProjects"))["items"] == projects_before
    assert json.loads(browser("eval", "document.querySelector('#project').value")) == str(project_id)
    print("PASS: canonical/private navigation order, real keyboard/pointer reorder, cancelled drag, left/right collapse, reload/mobile layout and grouped project creation without publication")
