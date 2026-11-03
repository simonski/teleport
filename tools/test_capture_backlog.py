"""Check capture retries, execution guards and parent preservation with a zero-server fixture."""
import copy
import json
import io
import subprocess
import unittest
from unittest.mock import patch

from capture_backlog import ROOT, capture, tk, validate


class Fixture:
    def __init__(self):
        self.records, self.receipts, self.calls = {}, {}, []
        self.projects = []
        self.agents = False

    def __call__(self, operation, data=None):
        self.calls.append((operation, copy.deepcopy(data)))
        if operation == "getSession":
            return {"installationId": "fixture", "settings": {"agents": self.agents, "executionMode": "off"}}
        if operation == "spec":
            return {"paths": {"/api/tools/" + name: {} for name in ("createProject", "createTicket", "getTicket", "updateTicket", "listProjects")}}
        if operation == "listProjects":
            return {"items": self.projects}
        if operation == "createProject":
            self.projects.append({**data, "id": 10})
            return self.projects[-1]
        if operation == "createTicket":
            if data["requestId"] in self.receipts:
                previous = self.receipts[data["requestId"]]
                if previous[0] != data:
                    raise ValueError("idempotency conflict")
                return copy.deepcopy(previous[1])
            ticket = {**data, "id": len(self.records) + 1, "version": 1, "status": "Todo", "labels": []}
            self.records[ticket["id"]] = ticket
            self.receipts[data["requestId"]] = (copy.deepcopy(data), copy.deepcopy(ticket))
            return copy.deepcopy(ticket)
        if operation == "getTicket":
            return copy.deepcopy(self.records[data["id"]])
        if operation == "updateTicket":
            ticket = self.records[data["id"]]
            if ticket["version"] != data["expectedVersion"]:
                raise ValueError("version conflict")
            ticket["labels"] = data["labels"]
            ticket["version"] += 1
            return copy.deepcopy(ticket)
        raise AssertionError(operation)


class CaptureTest(unittest.TestCase):
    def setUp(self):
        quiet = patch("sys.stdout", new=io.StringIO())
        quiet.start()
        self.addCleanup(quiet.stop)
        self.plan = json.loads((ROOT / "planning/roadmap.json").read_text())
        self.plan["stories"] = self.plan["stories"][:2]

    def test_retry_keeps_ids_user_edits_and_labels_without_duplicate_tickets(self):
        api = Fixture()
        first = capture(self.plan, api)
        story = api.records[first["stories"][0]["id"]]
        story["title"] = "A human refined this title"
        story["labels"].append("human-added")
        story["version"] += 1
        count = len(api.records)
        again = capture(self.plan, api)
        self.assertEqual(len(api.records), count)
        self.assertEqual(first["stories"][0]["id"], again["stories"][0]["id"])
        self.assertIn("human-added", story["labels"])
        self.assertEqual(story["title"], "A human refined this title")
        self.assertEqual(story["parentId"], first["epics"]["mvp"])
        self.assertTrue(story["agent"])

    def test_enabled_execution_and_demo_projects_block_capture_without_changes(self):
        api = Fixture()
        api.agents = True
        with self.assertRaises(RuntimeError):
            capture(self.plan, api)
        self.assertFalse(api.records or api.projects)
        api.agents = False
        api.projects = [{"id": 1, "key": "teleport", "simulation": True}]
        with self.assertRaises(RuntimeError):
            capture(self.plan, api)
        self.assertFalse(api.records)

    def test_moved_ticket_requires_reconciliation_instead_of_undoing_user_changes(self):
        api = Fixture()
        first = capture(self.plan, api)
        story = api.records[first["stories"][0]["id"]]
        story["parentId"] = 999
        with self.assertRaises(RuntimeError):
            capture(self.plan, api)
        self.assertEqual(story["parentId"], 999)

    def test_manifest_has_one_unique_story_per_area_and_keeps_deferred_topics_out_of_mvp(self):
        plan = json.loads((ROOT / "planning/roadmap.json").read_text())
        validate(plan)
        self.assertEqual(len(plan["stories"]), 29)
        stories = {story["key"]: story for story in plan["stories"]}
        for key in ("waitlist", "email-process", "email-daemon", "user-inspection"):
            self.assertEqual(stories[key]["phase"], "later")
        for key in ("workflow-graph", "ticket-context", "goal-lifecycle"):
            self.assertTrue(stories[key]["questions"])
        plan["stories"].append(plan["stories"][0])
        with self.assertRaises(ValueError):
            validate(plan)

    def test_cli_uses_structured_data_and_does_not_disclose_arguments_on_failure(self):
        data = {"title": "Private `literal` $(text) with a newline\n"}
        with patch("capture_backlog.subprocess.run", return_value=subprocess.CompletedProcess([], 0, '{"id":1}', "")) as run:
            self.assertEqual(tk("createTicket", data), {"id": 1})
        self.assertEqual(run.call_args.args[0][1:3], ["createTicket", "-data"])
        self.assertEqual(json.loads(run.call_args.args[0][3]), data)
        with patch("capture_backlog.subprocess.run", return_value=subprocess.CompletedProcess([], 1, "", "private failure")):
            with self.assertRaises(RuntimeError) as failure:
                tk("createTicket", data)
        self.assertNotIn("Private", str(failure.exception))
        self.assertNotIn("private failure", str(failure.exception))
        with patch("capture_backlog.subprocess.run", side_effect=subprocess.TimeoutExpired(["private argument"], 15)):
            with self.assertRaises(RuntimeError) as failure:
                tk("createTicket", data)
        self.assertNotIn("private argument", str(failure.exception))


if __name__ == "__main__":
    from tool_history import run
    run(unittest.main)
