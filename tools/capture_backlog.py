"""Capture a reviewed roadmap as inactive tickets through the authenticated tk API."""
import argparse
import json
from pathlib import Path
import subprocess
import uuid

ROOT = Path(__file__).resolve().parents[1]


def tk(operation, data=None):
    command = [str(ROOT / "bin/tk"), operation]
    if data is not None:
        command += ["-data", json.dumps(data)]
    command.append("-json")
    try:
        result = subprocess.run(command, capture_output=True, text=True, timeout=15)
    except subprocess.TimeoutExpired:
        raise RuntimeError(f"tk {operation} timed out; reconcile the result before retrying the same capture") from None
    if result.returncode:
        raise RuntimeError(f"tk {operation} failed with exit status {result.returncode}; inspect authentication or server diagnostics")
    return json.loads(result.stdout)


def validate(plan):
    if not plan.get("batch") or not plan.get("project", {}).get("key"):
        raise ValueError("A stable batch and project key are required")
    keys = set()
    for story in plan["stories"]:
        if not story.get("key") or story["key"] in keys or story.get("phase") not in plan["epics"]:
            raise ValueError("Story keys must be unique and phases must name a declared epic")
        keys.add(story["key"])
        if not story.get("title") or not story.get("scope") or not story.get("acceptance"):
            raise ValueError("Every story requires a title, scope and acceptance criteria")


def capture(plan, api=tk):
    validate(plan)
    session = api("getSession")
    def paused():
        settings = api("getSession")["settings"]
        if settings.get("agents") or settings.get("executionMode") not in (None, "", "off"):
            raise RuntimeError("Roadmap capture requires agent execution to be off; no settings were changed")
    paused()
    discovery = api("spec")
    for name in ("createProject", "createTicket", "getTicket", "updateTicket", "listProjects"):
        if "/api/tools/" + name not in discovery["paths"]:
            raise RuntimeError("The server does not expose the required capture operations")
    def request(key):
        return str(uuid.uuid5(uuid.NAMESPACE_URL, ":".join((session["installationId"], plan["batch"], plan["project"]["key"], key))))
    projects, cursor = [], ""
    while True:
        page = api("listProjects", {"limit": 200, "cursor": cursor})
        projects += page["items"]
        cursor = page.get("nextCursor")
        if not cursor:
            break
    project = next((item for item in projects if item["key"] == plan["project"]["key"]), None)
    if project is None:
        project = api("createProject", {**plan["project"], "requestId": request("project")})
    if project.get("simulation"):
        raise RuntimeError("Refusing to mix product roadmap tickets with a simulation project")

    def create(key, kind, title, description, criteria, parent=0, priority="medium", labels=()):
        paused()
        created = api("createTicket", {
            "projectId": project["id"], "parentId": parent, "kind": kind,
            "title": title, "description": description, "criteria": criteria,
            "agent": kind == "task", "priority": priority, "requestId": request(key),
        })
        # Creation receipts preserve the original response. Read current state on retries.
        current = api("getTicket", {"id": created["id"]})
        expected_parent = parent if kind != "goal" else 0
        if current["projectId"] != project["id"] or current["kind"] != kind or current["parentId"] != expected_parent:
            raise RuntimeError("A captured ticket has since moved; retain it for human reconciliation")
        desired = sorted(set(current.get("labels") or []) | set(labels))
        if desired != sorted(current.get("labels") or []):
            current = api("updateTicket", {"id": current["id"], "expectedVersion": current["version"], "labels": desired})
        return current

    goal = create("goal", "goal", plan["goal"]["title"], plan["goal"]["description"], plan["goal"]["criteria"])
    epics = {}
    for phase, epic in plan["epics"].items():
        epics[phase] = create("epic:" + phase, "epic", epic["title"], epic["description"], epic["criteria"], goal["id"], labels=("roadmap", phase))
    result = {"installationId": session["installationId"], "projectId": project["id"], "goalId": goal["id"], "epics": {key: value["id"] for key, value in epics.items()}, "stories": []}
    for story in plan["stories"]:
        description = "\n\n".join((
            "Planning capture only. Creation does not authorize execution, model spending, permission changes or deployment.",
            "Area: " + story["key"] + "\nPhase: " + story["phase"],
            "Requested scope:\n" + "\n".join("- " + item for item in story["scope"]),
            "Questions and constraints:\n" + "\n".join("- " + item for item in story.get("questions", [])) if story.get("questions") else "",
        )).strip()
        ticket = create("story:" + story["key"], "task", story["title"], description, "\n".join("- " + item for item in story["acceptance"]), epics[story["phase"]]["id"], story.get("priority", "high" if story["phase"] == "mvp" else "low"), ("roadmap", story["phase"], story["key"]))
        result["stories"].append({"key": story["key"], "id": ticket["id"], "title": ticket["title"], "status": ticket["status"]})
        print(f"#{ticket['id']} {story['phase']} · {ticket['title']}", flush=True)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("manifest", type=Path)
    parser.add_argument("--plan", action="store_true", help="Show the capture without calling the API")
    parser.add_argument("--receipt", type=Path, default=ROOT / ".cache/roadmap/capture.json")
    args = parser.parse_args()
    plan = json.loads(args.manifest.read_text())
    validate(plan)
    if args.plan:
        for story in plan["stories"]:
            print(story["phase"] + " · " + story["title"])
        return
    result = capture(plan)
    args.receipt.parent.mkdir(parents=True, exist_ok=True)
    args.receipt.write_text(json.dumps(result, indent=2) + "\n")
    print(f"Captured {len(result['stories'])} stories in project #{result['projectId']}; receipt: {args.receipt}")


if __name__ == "__main__":
    from tool_history import run
    run(main)
