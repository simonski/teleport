"""Verify refresh/sign-in restoration through the real browser and server."""


def verify_location(browser, api, project):
    browser("click", '[data-view="Ideas"]')
    browser("wait", "--fn", "view==='Ideas'&&!locationPromise")
    assert api("getSession")["lastLocation"] == {"view": "Ideas", "projectId": project["id"]}
    browser("reload")
    browser("wait", '#nav-views [data-view="Ideas"][aria-current="page"]')
    browser("eval", f"if(projectId!=={project['id']})throw new Error('Refresh lost project context')")
    browser("click", "#account")
    browser("click", "#menu-sign-out")
    browser("wait", '#login input[name="username"]')
    # Clear only this fixture's local context to prove server restoration.
    browser("eval", "for(const key of Object.keys(localStorage))if(key.startsWith('teleport-location:'))localStorage.removeItem(key)")
    browser("fill", 'input[name="username"]', "admin")
    browser("fill", 'input[name="password"]', "password")
    browser("click", "#login button")
    browser("wait", '#nav-views [data-view="Ideas"][aria-current="page"]')
    browser("eval", f"if(projectId!=={project['id']})throw new Error('Sign-in lost project context')")
    browser("screenshot", "/tmp/teleport-restored-location.png")
    print("PASS: remembered project/view after refresh and sign-out/sign-in, including server restoration with the local cache removed", flush=True)
