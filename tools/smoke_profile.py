"""Safe stage and action timings for acceptance runs, without input values."""
from collections import defaultdict
import csv
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import time
import sys
import uuid

from tool_history import locked_history

ROOT = Path(__file__).resolve().parents[1]
FIELDS = ("run_id", "profile", "stage", "category", "calls", "started_at", "finished_at", "duration_seconds", "outcome")


class Profile:
    def __init__(self, mode, folder):
        self.mode, self.folder = mode, folder
        self.identity = os.environ.get("TELEPORT_TOOL_RUN_ID") or str(uuid.uuid4())
        self.path = Path(os.environ.get("TELEPORT_TOOL_STAGES", ROOT / "tool_calls_stages.csv"))
        self.stage, self.counts = None, defaultdict(lambda: [0, 0.0])

    def start(self, stage):
        self.finish("failure" if sys.exc_info()[0] else "success")
        self.stage, self.started, self.clock = stage, datetime.now(timezone.utc), time.monotonic()
        (self.folder / "smoke-progress.json").write_text(json.dumps({
            "stage": stage, "profile": self.mode, "run_id": self.identity,
            "started_at": self.started.isoformat(), "clock": self.clock,
        }))

    def call(self, category, action):
        clock = time.monotonic()
        try:
            return action()
        finally:
            self.counts[category][0] += 1
            self.counts[category][1] += time.monotonic() - clock

    def finish(self, outcome="success"):
        if self.stage is None:
            return
        ended = datetime.now(timezone.utc)
        elapsed = time.monotonic() - self.clock
        counts = {"stage": [1, elapsed], **self.counts}
        with locked_history(self.path):
            with self.path.open("a", newline="", encoding="utf-8") as output:
                writer = csv.writer(output)
                if output.tell() == 0:
                    writer.writerow(FIELDS)
                for category, (count, duration) in counts.items():
                    writer.writerow((self.identity, self.mode, self.stage, category, count,
                                     self.started.isoformat(), ended.isoformat(), f"{duration:.6f}", outcome))
        print(f"STAGE {self.stage}: {elapsed:.3f}s ({outcome})", flush=True)
        (self.folder / "smoke-progress.json").unlink(missing_ok=True)
        self.stage, self.counts = None, defaultdict(lambda: [0, 0.0])


def record_interruption(folder, outcome):
    progress = folder / "smoke-progress.json"
    if not progress.exists():
        return
    state = json.loads(progress.read_text())
    print("Last smoke stage: " + state["stage"], flush=True)
    profile = Profile(state["profile"], folder)
    profile.identity, profile.stage = state["run_id"], state["stage"]
    profile.started = datetime.fromisoformat(state["started_at"])
    profile.clock = state["clock"]
    profile.finish(outcome)


def record_timeout(folder):
    record_interruption(folder, "timeout")
