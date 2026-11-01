"""Real-browser checks for view-aware creation and ticket keyboard ownership."""
import json


def verify_ticket_keyboard(browser, tk, project_id):
    def call(operation, data):
        return json.loads(tk(operation, "--data", json.dumps(data)))

    def focused(ticket):
        browser("eval", f"if(document.activeElement?.dataset.ticket!=={json.dumps(str(ticket['id']))})throw new Error('Wrong ticket focus')")

    before = call("listTickets", {"projectId": project_id})["items"]
    for view, kind, intent in [("Goals", "goal", "agent"), ("Epics", "epic", "agent"), ("Todos", "task", "human")]:
        browser("click", f'#nav-views [data-view="{view}"]')
        for trigger in ["button", "key"]:
            if trigger == "button":
                browser("click", "#new")
            else:
                browser("press", "n")
            browser("wait", '#editor [name="title"]')
            browser("eval", f"if(document.querySelector('[name=kind]').value!=={json.dumps(kind)}||document.querySelector('[name=intent]').value!=={json.dumps(intent)}||document.activeElement!==document.querySelector('[name=title]'))throw new Error('Wrong creation context or focus')")
            browser("press", "n")
            browser("eval", "if(document.querySelector('[name=title]').value!=='n')throw new Error('Shortcut stole editor text input')")
            browser("press", "Escape")
    assert call("listTickets", {"projectId": project_id})["items"] == before

    created = [call("createTicket", {"projectId": project_id, "title": title, "agent": False, "requestId": "keyboard-" + str(index)}) for index, title in enumerate(["Keyboard first", "Keyboard second", "Keyboard next lane"])]
    created[2] = call("transitionTicket", {"id": created[2]["id"], "expectedVersion": created[2]["version"], "target": "Doing"})
    browser("click", '#nav-views [data-view="Board"]')
    browser("select", "#lens", "human")
    browser("click", "#new")
    browser("eval", "if(document.querySelector('[name=kind]').value!=='task'||document.querySelector('[name=intent]').value!=='human')throw new Error('Board lens ignored')")
    browser("press", "Escape")
    browser("wait", f'.ticket-open[data-ticket="{created[0]["id"]}"]')
    browser("eval", f'document.querySelector(\'.ticket-open[data-ticket="{created[0]["id"]}"]\').focus()')
    browser("press", "ArrowDown"); focused(created[1])
    browser("press", "ArrowDown"); focused(created[1])
    browser("press", "ArrowRight"); focused(created[2])
    browser("press", "ArrowLeft"); focused(created[1])
    browser("press", "ArrowUp"); focused(created[0])
    changed = call("updateTicket", {"id": created[0]["id"], "expectedVersion": created[0]["version"], "priority": "high"})
    created[0] = changed
    browser("wait", "--fn", f'tickets.some(t=>t.id==={changed["id"]}&&t.version==={changed["version"]})')
    focused(created[0])
    browser("screenshot", "/tmp/teleport-board-keyboard.png")
    browser("press", "e")
    browser("wait", '#editor [name="title"]')
    browser("eval", "if(document.querySelector('#editor [name=title]').value!=='Keyboard first')throw new Error('E opened a different record')")
    browser("press", "Escape"); focused(created[0])
    for ticket in created:
        assert call("getTicket", {"id": ticket["id"]})["version"] == ticket["version"], "Keyboard navigation mutated work"
        call("setTicketVisibility", {"id": ticket["id"], "expectedVersion": ticket["version"], "archived": True})
    browser("click", '#nav-views [data-view="Board"]')
    browser("select", "#lens", "agent")
    browser("click", '#nav-views [data-view="Home"]')
    print("PASS: view-aware New/N, cancelled creation, editor input ownership, Board lens defaults, arrow focus, refresh preservation and exact E edit without ticket-state mutation")
