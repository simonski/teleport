"""Verify human-only Todos, queue handoff and CSS calendars in a real browser."""
import datetime


def verify_human_todos(browser, api, project):
    project = api("setHandbrake", {"projectId": project["id"], "expectedVersion": project["version"], "engaged": True})
    ticket = api("listTickets", {"projectId": project["id"], "kind": "task", "agent": False, "status": "Todo"})["items"][0]
    browser("click", '[data-view="Todos"]')
    card = f'[data-ticket-card="{ticket["id"]}"]'
    button = card + ' .ticket-open'
    browser("wait", button)
    browser("eval", "if([...document.querySelectorAll('.human-board [data-ticket-card]')].some(card=>tickets.find(ticket=>ticket.id===Number(card.dataset.ticketCard)).agent))throw new Error('Agent ticket leaked into Todos')")
    browser("click", button)
    browser("wait", ".date-trigger")
    browser("eval", "if(document.querySelector('#editor .native-session-panel')||document.querySelectorAll('#editor .date-trigger').length!==3)throw new Error('Human editor exposes harnesses or lacks calendars')")
    browser("fill", 'input[name="due"]', "2030-03-15")
    browser("click", '[aria-label="Choose due date"]')
    browser("wait", '[data-date="2030-03-15"]')
    browser("press", "ArrowRight")
    browser("eval", "if(document.activeElement.dataset.date!=='2030-03-16')throw new Error('Calendar arrow did not move one day')")
    browser("screenshot", "/tmp/teleport-date-picker.png")
    browser("press", "Escape")
    browser("eval", "if(!document.querySelector('#editor').open||document.querySelector('.date-picker'))throw new Error('Calendar Escape dismissed its owning editor')")
    today = datetime.date.today().isoformat()
    for field in ["start", "eta", "due"]:
        browser("scrollintoview", f'[aria-label="Choose {field} date"]')
        browser("click", f'[aria-label="Choose {field} date"]')
        browser("find", "role", "button", "click", "--name", "Today", "--exact")
        assert today in browser("get", "value", f'input[name="{field}"]')
    browser("scrollintoview", '#editor button[type="submit"]')
    browser("click", '#editor button[type="submit"]')
    browser("wait", "--fn", "!document.querySelector('#editor').open")
    updated = api("getTicket", {"id": ticket["id"]})
    assert updated["start"] == updated["eta"] == updated["due"] == today

    def menu():
        browser("scrollintoview", button)
        browser("focus", button)
        browser("press", "Shift+F10")
        browser("wait", ".ticket-context-menu")

    menu()
    browser("click", '[data-ticket-menu="move-Doing"]')
    browser("wait", "--fn", f"tickets.find(ticket=>ticket.id==={ticket['id']})?.status==='Doing'")
    browser("drag", card, '.human-board .lane:nth-child(1) .lane-body')
    browser("wait", "--fn", f"tickets.find(ticket=>ticket.id==={ticket['id']})?.status==='Todo'")
    menu()
    browser("click", '[data-ticket-menu="move-Done"]')
    browser("wait", "#agreement")
    assert api("getTicket", {"id": ticket["id"]})["status"] == "Todo"
    browser("check", "#agreement")
    browser("click", '#editor button[type="submit"]')
    browser("wait", "--fn", f"tickets.find(ticket=>ticket.id==={ticket['id']})?.status==='Done'")
    browser("screenshot", "/tmp/teleport-human-todos.png")
    queued = api("createTicket", {"projectId": project["id"], "title": "Queue from human Todos", "agent": False, "requestId": "browser-queue"})
    button = f'[data-ticket="{queued["id"]}"]'
    browser("wait", button)
    menu()
    browser("click", '[data-ticket-menu="refine"]')
    browser("wait", "--fn", f"!document.querySelector('.human-board [data-ticket=\"{queued['id']}\"]')")
    assert api("getTicket", {"id": queued["id"]})["agent"]
    browser("click", '[data-view="Board"]')
    browser("wait", button)
    menu()
    browser("click", '[data-ticket-menu="return-todo"]')
    browser("click", '[data-view="Todos"]')
    browser("wait", button)
    assert not api("getTicket", {"id": queued["id"]})["agent"]
    print("PASS: human-only Todos, harness-free editor, three CSS/keyboard calendars, saved dates, context moves, real drag, human Done confirmation and refinement queue/return", flush=True)
