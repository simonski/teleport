"""Real-browser checks for landing flight and modal keyboard ownership."""
import json


def verify_landing(browser):
    browser("wait", ".login-scene")
    browser("wait", "--fn", "typeof landingFlight!=='undefined'&&!!landingFlight")
    browser("eval", "if(document.querySelector('.login-scene rect,.login-scene foreignObject')||getComputedStyle(document.querySelector('.login-brand')).backgroundColor!=='rgba(0, 0, 0, 0)')throw new Error('Landing flight has an opaque rectangular mask')")
    states = json.loads(browser("eval", "(()=>{const times=[0,2875,4300,7000,10000,16000,19000,21000];return times.map(time=>{const state=landingFlight.seek(time,0);const box=document.querySelector('.flight-rocket use').getBoundingClientRect();return {...state,screenX:box.x,screenY:box.y};});})()"))
    assert states[0]["phase"] == "rest" and states[0]["engine"] == 0
    assert states[1]["phase"] == "ignite" and 0 < abs(states[1]["angle"] + 90) < 3
    assert max(point["screenX"] for point in states) - min(point["screenX"] for point in states) > 200
    assert any(point["phase"] == "orbit" and point["engine"] == 0 for point in states)
    for name, at in [("ignition", 2875), ("orbit", 10000), ("catch", 19900), ("smoke", 20800)]:
        browser("eval", f"landingFlight.seek({at},0)")
        browser("screenshot", "/tmp/teleport-landing-" + name + ".png")
    browser("eval", "landingFlight.seek(10000,0)")
    original = json.loads(browser("eval", "landingFlight.snapshot()"))
    point = json.loads(browser("eval", "(()=>{const r=document.querySelector('.login-moon').getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()"))
    browser("mouse", "move", str(point["x"]), str(point["y"]))
    browser("mouse", "down", "left")
    browser("mouse", "move", str(point["x"]-65), str(point["y"]+30))
    browser("mouse", "up", "left")
    moved = json.loads(browser("eval", "landingFlight.snapshot()"))
    assert moved["moon"]["x"] < original["moon"]["x"] - 20 and moved["moon"]["y"] > original["moon"]["y"]
    assert abs((moved["x"]-original["x"]) - (moved["moon"]["x"]-original["moon"]["x"])) < .01
    browser("set", "viewport", "390", "844")
    browser("eval", "landingFlight.seek(0,0);if(document.documentElement.scrollWidth>innerWidth)throw new Error('Landing animation overflows mobile')")
    browser("screenshot", "/tmp/teleport-landing-mobile.png")
    browser("set", "media", "light", "reduced-motion")
    browser("eval", "if(document.querySelectorAll('.flight-rocket').length!==1||document.querySelector('.login-scene').dataset.engine!=='0'||document.querySelector('.flight-rocket').getAnimations().length)throw new Error('Reduced motion is not one landed rocket')")
    browser("screenshot", "/tmp/teleport-landing-static.png")
    browser("set", "media", "light")
    browser("set", "viewport", "1280", "800")
    print("PASS: supported launch, preflight wobble, ignition/coast/return/catch/smoke, draggable moon retargeting, mobile bounds and reduced motion")


def verify_profile_dialogs(browser):
    browser("eval", "window.popupUnderlying=document.querySelector('main');window.popupView=view;window.modalKeyLeaks=0;window.modalLeakedKey=()=>window.modalKeyLeaks++;['keydown','keyup','keypress'].forEach(type=>document.addEventListener(type,window.modalLeakedKey));")
    for name in ("Settings", "System", "Project settings"):
        browser("find", "role", "button", "click", "--name", name, "--exact")
        browser("eval", "{const dialog=document.querySelector('tk-dialog[open]'),shade=dialog?.closest('.modal-shade');if(!shade||!document.querySelector('#app').inert||document.querySelector('main')!==window.popupUnderlying||view!==window.popupView||getComputedStyle(shade).backdropFilter!=='blur(6px)')throw new Error('Profile selection is not a blurred popup over the existing view');window.modalKeyLeaks=0;}")
        for key in ("q", "n", "e", "/", "`", "ArrowDown", "Shift", "Shift"):
            browser("press", key)
        browser("eval", "if(window.modalKeyLeaks||view!==window.popupView||document.querySelector('#finder-query,#script-source,#new-console'))throw new Error('Dialog keys escaped to an application shortcut')")
        browser("eval", "{const dialog=document.querySelector('tk-dialog[open]'),items=modalFocusables(dialog);items.at(-1).focus();}")
        browser("press", "Tab")
        browser("eval", "{const dialog=document.querySelector('tk-dialog[open]');if(document.activeElement!==modalFocusables(dialog)[0])throw new Error('Tab escaped dialog');}")
        browser("press", "Shift+Tab")
        browser("eval", "{const dialog=document.querySelector('tk-dialog[open]');if(document.activeElement!==modalFocusables(dialog).at(-1))throw new Error('Shift+Tab escaped dialog');}")
        if name == "Settings":
            browser("click", "#appearance-style + .choice-button")
            browser("eval", "document.querySelector('#appearance-style + .choice-button').focus({preventScroll:true})")
            browser("press", "ArrowDown")
            browser("eval", "if(!document.querySelector('.choice-list')||!openChoice)throw new Error('Arrow on an open combobox toggled it closed')")
            browser("press", "Escape")
            browser("eval", "if(document.querySelector('.choice-list')||!document.querySelector('tk-dialog[open]'))throw new Error('Selector Escape closed its parent dialog')")
        browser("screenshot", "/tmp/teleport-popup-" + name.lower().replace(" ", "-") + ".png")
        if name == "System":
            browser("mouse", "move", "8", "8"); browser("mouse", "down", "left"); browser("mouse", "up", "left")
        else:
            browser("press", "Escape")
        browser("eval", "if(document.querySelector('tk-dialog[open]')||document.querySelector('#app').inert||document.activeElement!==document.querySelector('#account')||document.querySelector('main')!==window.popupUnderlying)throw new Error('Popup dismissal changed the view or lost focus')")
    browser("click", "#new")
    browser("eval", "window.modalKeyLeaks=0")
    for key in ("n", "q", "/", "`", "e"):
        browser("press", key)
    browser("eval", "if(document.querySelector('#editor [name=title]').value!=='nq/`e'||window.modalKeyLeaks||view!==window.popupView)throw new Error('Keyboard containment swallowed local editing or leaked shortcuts')")
    browser("press", "Escape")
    browser("eval", "['keydown','keyup','keypress'].forEach(type=>document.removeEventListener(type,window.modalLeakedKey))")
    # Check content directly at every usable view, including Graph's own header.
    for name in ("Home", "Ideas", "Goals", "Epics", "Board", "Todos", "Calendar", "Graph", "Cost", "Projects", "Workflows"):
        selector = '#nav-views [data-view=' + json.dumps(name) + ']'
        browser("eval", "document.querySelector(" + json.dumps(selector) + ").click();if(document.querySelector('main h1,.page-heading .eyebrow,.board-heading .eyebrow,.graph-heading .eyebrow')||/WORKSPACE \\/|RELATIONSHIPS \\/|PLANNING \\/|DELIVERY \\/|USAGE \\//.test(document.querySelector('main').innerText))throw new Error('Redundant view breadcrumb remains');{const main=document.querySelector('main');document.querySelector('#account').click();document.querySelector('#account-settings').click();if(!document.querySelector('tk-dialog[open]')?.closest('.modal-shade')||document.querySelector('main')!==main)throw new Error('Settings became a page on this view');}")
        browser("press", "Escape")
    browser("click", '#nav-views [data-view="Home"]')
    print("PASS: profile popups preserve each view, blur background, contain all application keys/Tab, and omit redundant breadcrumb rows")
