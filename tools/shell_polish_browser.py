"""Real-browser checks for landing flight and modal keyboard ownership."""
import json


def verify_landing(browser):
    browser("wait", ".login-scene")
    browser("wait", "--fn", "typeof landingFlight!=='undefined'&&!!landingFlight")
    browser("eval", "if(document.querySelector('.login-scene rect,.login-scene foreignObject')||getComputedStyle(document.querySelector('.login-brand')).backgroundColor!=='rgba(0, 0, 0, 0)')throw new Error('Landing flight has an opaque rectangular mask')")
    states = json.loads(browser("eval", "(()=>{return ['rest','ignite','lift','outbound','orbit','return','descend','smoke'].map(name=>{const state=landingFlight.seekPhase(name,name==='ignite'?.375:.5,0);const box=document.querySelector('.rocket-body').getBoundingClientRect();return {...state,screenX:box.x,screenY:box.y};});})()"))
    assert states[0]["phase"] == "rest" and states[0]["engine"] == 0 and states[0]["fins"] > .9
    assert states[1]["phase"] == "ignite" and 0 < abs(states[1]["angle"] + 90) < 3
    assert max(point["screenX"] for point in states) - min(point["screenX"] for point in states) > 200
    assert any(point["phase"] == "orbit" and point["engine"] == 0 for point in states)
    browser("eval", "{const frame=landingFlight.seekPhase('delivery',.8,0);if(frame.vehicle.kind!=='carrier'||frame.x!==frame.vehicle.x)throw new Error('Rocket is not being transported');}")
    browser("screenshot", "/tmp/teleport-landing-transport.png")
    browser("eval", "{landingFlight.seekPhase('refuel',.6,0);const opacity=selector=>[...document.querySelectorAll(selector)].some(node=>Number(getComputedStyle(node).opacity)>0);if(!opacity('.fuel-steam circle')||!opacity('.fuel-hose')||document.querySelector('.login-scene').dataset.engine!=='0')throw new Error('Refuelling steam/hose is missing');}")
    browser("screenshot", "/tmp/teleport-landing-refuelling.png")
    browser("eval", "{landingFlight.seekPhase('fault',.8,2);if(Number(getComputedStyle(document.querySelector('.rocket-debris')).opacity)===0)throw new Error('Fallen panel is missing');}")
    browser("screenshot", "/tmp/teleport-landing-repair.png")
    browser("eval", "{const frame=landingFlight.seekPhase('recovery-exit',.25,2);if(frame.vehicle.kind!=='recovery'||!frame.broken)throw new Error('Recovery truck did not collect damaged rocket');}")
    browser("screenshot", "/tmp/teleport-landing-recovery.png")
    fleet = json.loads(browser("eval", "(()=>{return Array.from({length:6},(_,index)=>{const frame=landingFlight.seekPhase('rest',.5,index*3);const body=[...document.querySelectorAll('.rocket-variant')].filter(node=>getComputedStyle(node).display!=='none');if(body.length!==1||body[0].getBoundingClientRect().height<10)throw new Error('Invalid visible rocket');return {id:document.querySelector('.login-scene').dataset.rocket,hull:body[0].querySelector('.rocket-hull').getAttribute('d')};});})()"))
    assert len({item["id"] for item in fleet}) == 6 and len({item["hull"] for item in fleet}) == 6
    browser("screenshot", "/tmp/teleport-landing-starship.png")
    for name, phase, fraction in [("ignition", "ignite", .65), ("orbit", "orbit", .5), ("catch", "descend", .9), ("smoke", "smoke", .5)]:
        browser("eval", f"landingFlight.seekPhase({json.dumps(phase)},{fraction},0)")
        if phase == "ignite":
            browser("eval", "if(![...document.querySelectorAll('.landing-smoke circle')].some(node=>Number(getComputedStyle(node).opacity)>0))throw new Error('Takeoff smoke is missing')")
        browser("screenshot", "/tmp/teleport-landing-" + name + ".png")
    browser("eval", "landingFlight.seekPhase('orbit',.5,0)")
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
    browser("eval", "landingFlight.seekPhase('refuel',.5,15);if(document.documentElement.scrollWidth>innerWidth)throw new Error('Landing animation overflows mobile')")
    browser("screenshot", "/tmp/teleport-landing-mobile.png")
    browser("set", "media", "light", "reduced-motion")
    browser("eval", "{const state=landingFlight.snapshot();if(document.querySelectorAll('.flight-rocket').length!==1||state.phase!=='rest'||state.vehicle||state.steam||state.engine||document.querySelector('.flight-rocket').getAnimations().length||getComputedStyle(document.querySelector('.ground-vehicle')).display!=='none')throw new Error('Reduced motion is not one landed rocket');}")
    browser("screenshot", "/tmp/teleport-landing-static.png")
    browser("set", "media", "light")
    browser("set", "viewport", "1280", "800")
    print("PASS: carrier delivery, tanker/hose/filling/steam, recovery and six rockets, folding fins, takeoff/landing smoke, coast/catch, moon retargeting, mobile and reduced motion")


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
