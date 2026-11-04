"""Check current repository documentation and update CLI terminology.

Usage: python3 tools/spec_sources.py [--verify]
       python3 tools/spec_sources.py --rename-cli OLD NEW

Checks are read-only and run by default. CLI renaming updates SPECIFICATION.md.
"""

import argparse
import re
from pathlib import Path


def verify(root, document=None):
    specification = (document or root / "SPECIFICATION.md").read_text()
    failures = []

    def definitions(text, prefix):
        return re.findall(rf"^\| ({prefix}-?\d+) \|", text, re.M)

    actual_actions = definitions(specification, "ACT")
    if not actual_actions:
        failures.append("Missing action definitions")
    if len(actual_actions) != len(set(actual_actions)):
        failures.append("Duplicate action definitions")
    for prefix, expected in (("A", 186), ("D", 56), ("C", 9)):
        defined = definitions(specification, prefix)
        required = {f"{prefix}{number:02d}" for number in range(1, expected + 1)}
        if set(defined) != required or len(defined) != expected:
            failures.append(f"Invalid {prefix} definition set")
        for reference in re.findall(rf"\b{prefix}\d{{2,}}\b", specification):
            if reference not in required:
                failures.append(f"Undefined reference {reference}")
    for reference in re.findall(r"\bACT-\d{4}\b", specification):
        if reference not in set(actual_actions):
            failures.append(f"Undefined action reference {reference}")
    headings = re.findall(r"^#{1,6} (.+)$", specification, re.M)
    anchors = {re.sub(r"[^\w\- ]", "", heading.lower()).replace(" ", "-")
               for heading in headings}
    for target in re.findall(r"\]\(([^)]+)\)", specification):
        if target.startswith("#"):
            if target[1:] not in anchors:
                failures.append(f"Missing heading target {target}")
        elif not re.match(r"https?://", target) and not (root / target).is_file():
            failures.append(f"Missing local link {target}")
    numbered_sections = set(re.findall(r"^#{2,3} (\d+(?:\.\d+)?)[. ]", specification, re.M))
    for reference in re.findall(r"\bsections? (\d+(?:\.\d+)?)", specification):
        if reference not in numbered_sections:
            failures.append(f"Undefined section reference {reference}")
    planned_documents = {"README.md", "USER_GUIDE.md", "SKILL.md", ".agents/skills/teleport/SKILL.md"}
    published_documents = {"/skill.md"}
    for name in ("AGENTS.md", "SDLC.md", "SPECIFICATION.md"):
        text = specification if name == "SPECIFICATION.md" else (root / name).read_text()
        for target in re.findall(r"`([^`\s]+\.md)`", text):
            if target not in planned_documents | published_documents and not (root / target).is_file():
                failures.append(f"{name} references missing document {target}")
    forbidden = (
        r"\bStory\s+#\d+\b",
        r"\bticket\s+#\d+\b",
        r"\bsource (?:stories|archive|documents|authority)\b",
        r"\b(?:inherited catalogue|legacy implementation|captured state)\b",
        r"Appendix B\b",
    )
    for pattern in forbidden:
        if re.search(pattern, specification, re.I):
            failures.append(f"Specification retains implementation provenance: {pattern}")
    if specification.count("```") % 2:
        failures.append("Unbalanced code fences")
    if len(re.findall(r"^# ", specification, re.M)) != 1:
        failures.append("Expected exactly one document title")
    if re.search(r"\btp\b|\bHuman board\b|\bAgent Costs\b|\bOverview\b", specification):
        failures.append("Obsolete CLI or view naming remains")
    for name in ("Home", "Backlog", "Board", "Todos", "Graph", "Cost", "Settings", "System", "Workflows", "Evolution"):
        if not re.search(rf"^\| {name} \|", specification, re.M):
            failures.append(f"Missing canonical view definition: {name}")
    workflow_requirements = (
        "**directed workflow graph**",
        "Backward edges, cycles and explicit same-phase retry edges are permitted",
        "### 4.6 Visual workflow editor",
        "**Todo → Doing → Done**",
        "**Todo → Typo → Refine → Ready → Implementation → Verify → Approval → Done**",
        "Review is agentic analysis; Approval is the human decision",
    )
    for requirement in workflow_requirements:
        if requirement not in specification:
            failures.append(f"Missing workflow requirement: {requirement}")
    for name in ("SPECIFICATION.md", "AGENTS.md"):
        text = specification if name == "SPECIFICATION.md" else (root / name).read_text()
        for obsolete in ("directed acyclic graph", "DAG-preserving",
                         "valid DAG", "form a DAG", "workflow DAG",
                         "D42 is an open product decision", "Approval approval"):
            if obsolete.lower() in text.lower():
                failures.append(f"{name} retains obsolete workflow wording: {obsolete}")
    prose = specification.replace("I/O", "input/output")
    if re.search(r"\b(I|me|my|we|us|our)\b", prose):
        # Literal documented 'me' is an application identity selector.
        matches = set(re.findall(r"\b(I|me|my|we|us|our)\b", prose)) - {"me"}
        if matches:
            failures.append(f"First-person prose violates SDLC: {sorted(matches)}")
    if failures:
        raise SystemExit("FAILED:\n" + "\n".join(failures))
    print(f"PASS: {len(actual_actions)} unique action definitions with valid references")
    print("PASS: A01–A186, D01–D56, and C01–C09 defined with valid references")
    print("PASS: current document references exist; planned/API documents are explicit")
    print("PASS: directed workflow graph, GUI editor, seeded defaults and review-role wording")
    print("PASS: standalone product specification contains no ticket/source provenance")
    print("PASS: tk CLI and canonical view naming")
    print("PASS: internal/local links, title, code fences, and SDLC prose checks")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    operations = parser.add_mutually_exclusive_group()
    operations.add_argument("--verify", action="store_true", help="Run read-only checks (default)")
    operations.add_argument("--rename-cli", nargs=2, metavar=("OLD", "NEW"),
                            help="Rename a CLI token consistently in SPECIFICATION.md")
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    if args.rename_cli:
        old, new = args.rename_cli
        if not all(re.fullmatch(r"[a-z][a-z0-9-]*", name) for name in (old, new)):
            raise SystemExit("CLI names must be lowercase executable names")
        destination = root / "SPECIFICATION.md"
        updated, count = re.subn(rf"\b{re.escape(old)}\b", new, destination.read_text())
        if count:
            destination.write_text(updated)
        print(f"Updated {count} CLI-name occurrences in SPECIFICATION.md")
        return
    verify(root)


if __name__ == "__main__":
    from tool_history import run
    run(main)
