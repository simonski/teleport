"""Exercise prompt commands through real isolated runs, never a host shell."""
import json


def verify_script_prompt(browser, tk, project_id):
    browser("press", "q")
    browser("eval", "{const tab=document.querySelector('#nav-views [aria-current=page]'),style=getComputedStyle(tab);if(document.activeElement!==tab||style.outlineStyle!=='none'||!style.boxShadow.includes('inset'))throw new Error('Q retains a side outline instead of an underline');}")
    browser("screenshot", "/tmp/teleport-navigation-underline.png")
    browser("press", "/")
    browser("wait", "#script-source")

    def command(source, expected="completed"):
        browser("wait", "--fn", "!document.querySelector('#script-submit').disabled")
        browser("fill", "#script-source", source)
        browser("press", "Enter")
        ready = "" if expected == "running" else "&&!document.querySelector('#script-submit').disabled"
        browser("wait", "--fn", "!document.querySelector('#script-source').value&&document.querySelector('#script-status').textContent.includes(" + json.dumps(expected) + ")" + ready)

    command("1 + 2")
    browser("eval", "if(document.querySelector('.script-entry:last-child .script-entry-output').textContent!=='3'||document.querySelector('#script-source').value)throw new Error('Expression prompt did not return a value and clear its line')")
    browser("press", "ArrowUp")
    assert "1 + 2" in browser("get", "value", "#script-source")
    browser("press", "ArrowDown")
    assert json.loads(browser("eval", "document.querySelector('#script-source').value")) == ""
    count = len(tk("listScriptRuns")["items"])
    browser("fill", "#script-source", "const x = 6;\nreturn x * 7;")
    assert len(tk("listScriptRuns")["items"]) == count, "Pasting executed a command"
    browser("press", "Enter")
    browser("wait", "--fn", "document.querySelector('.script-entry:last-child .script-entry-output')?.textContent==='42'")
    command("tk listProjects")
    browser("eval", "if(!document.querySelector('.script-entry:last-child .script-entry-output').textContent.includes('Script proof'))throw new Error('tk command did not use the API binding')")
    count = len(tk("listScriptRuns")["items"])
    browser("fill", "#script-source", "help"); browser("press", "Enter")
    assert len(tk("listScriptRuns")["items"]) == count
    browser("eval", "if(!document.querySelector('#script-result').textContent.includes('Shift+Enter'))throw new Error('Console help is missing')")
    browser("eval", "{const original=window.fetch;let lose=true;window.fetch=async(...args)=>{const response=await original(...args);if(lose&&String(args[0]).endsWith('/startScriptRun')){lose=false;throw new Error('Simulated lost command acknowledgement');}return response;};}")
    payload = {"projectId": project_id, "title": "Prompt receipt once", "agent": False, "requestId": "prompt-receipt-once"}
    browser("fill", "#script-source", "tk createTicket --data '" + json.dumps(payload) + "'")
    browser("press", "Enter")
    browser("wait", "--fn", "!document.querySelector('#script-retry').hidden&&!document.querySelector('#script-retry').disabled")
    browser("fill", "#script-source", "Preserve a newer draft")
    browser("click", "#script-retry")
    browser("wait", "--fn", "document.querySelector('#script-status').textContent.includes('completed')&&!document.querySelector('#script-submit').disabled")
    assert "Preserve a newer draft" in browser("get", "value", "#script-source")
    created = [ticket for ticket in tk("listTickets", {"projectId": project_id})["items"] if ticket["title"] == payload["title"]]
    assert len(created) == 1, "Uncertain submission duplicated an API effect"
    browser("screenshot", "/tmp/teleport-command-console.png")
    browser("set", "viewport", "390", "844")
    browser("eval", "if(document.documentElement.scrollWidth>innerWidth||document.querySelector('#script-source').getBoundingClientRect().bottom>innerHeight)throw new Error('Prompt is inaccessible on mobile')")
    browser("screenshot", "/tmp/teleport-command-console-mobile.png")
    browser("set", "viewport", "1280", "800")
    browser("fill", "#script-source", "clear"); browser("press", "Enter")
    browser("wait", "--fn", "!document.querySelector('#script-result').children.length")
    browser("press", "Escape")
    browser("press", "/")
    browser("wait", "#script-source")
    browser("eval", "if(!document.querySelector('#script-status').textContent.includes('completed'))throw new Error('Reopening lost the durable command')")
    command("while (true) {}", "running")
    browser("press", "Control+c")
    browser("wait", "--fn", "document.querySelector('#editor h2')?.textContent.startsWith('Stop J')")
    browser("click", '#editor button[type="submit"]')
    browser("wait", "--fn", "document.querySelector('#script-status')?.textContent.includes('stopped')&&!document.querySelector('#script-submit').disabled")
    browser("press", "Escape")
    print("PASS: Q underline, prompt expressions/multiline/tk commands, history, safe paste, local help/clear, lost-ack receipt retry preserving newer input, mobile and reconnect", flush=True)
