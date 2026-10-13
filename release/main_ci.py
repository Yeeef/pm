#!/usr/bin/env python3
"""Wait for main's CI on a commit, for the release workflow (release/README.md). Standard library only.

  main_ci.py SHA    exit 0 once every workflow run that a push of SHA started (gh run list --commit SHA --event push),
                    the release workflow's own left out, completed and passed, and every workflow that runs on each
                    push to main (.github/workflows, a push trigger on main without paths) has a run there; exit 1
                    at once when one failed or was cancelled, naming it, or when the deadline passes.

A release ships what main holds at SHA, so it waits for the checks main runs on SHA itself, not only the ones its PR
ran: a merge made outside make merge-ready is caught here.
"""

from __future__ import annotations

import json
import subprocess
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
RELEASE = "pm release"  # this workflow, which waits here and is not judged
PASSED = {"success", "skipped", "neutral"}
DEADLINE = 45 * 60  # seconds; main's CI takes about 10 minutes
POLL = 20  # seconds between reads of the runs


def push_workflows(root: Path) -> set[str]:
    """The names of the workflows that run on every push to main: a push trigger whose branches hold main, with no
    paths filter. The release workflow is left out."""
    names = set()
    for f in sorted((root / ".github" / "workflows").glob("*.yml")):
        lines = f.read_text().splitlines()
        name = next((line[6:].strip() for line in lines if line.startswith("name: ")), None)
        if "  push:" not in lines or name == RELEASE:
            continue
        i = lines.index("  push:") + 1
        block = []
        while i < len(lines) and (lines[i].startswith("    ") or not lines[i].strip()):
            block.append(lines[i].strip())
            i += 1
        if "branches: [main]" in block and not any(b.startswith("paths") for b in block):
            names.add(name)
    return names


def verdict(runs: list[dict], required: set[str]) -> tuple[list[str], list[str]]:
    """(failed, pending): each run that completed without passing, and each run still going or required workflow
    with no run yet. A workflow re-run keeps its run id, so gh reports its latest attempt."""
    failed, pending = [], []
    seen = set()
    for r in sorted(runs, key=lambda r: r.get("databaseId", 0)):
        name = r.get("workflowName")
        if name == RELEASE:
            continue
        seen.add(name)
        if r.get("status") != "completed":
            pending.append(f"{name} is {r.get('status')}")
        elif r.get("conclusion") not in PASSED:
            failed.append(f"{name} concluded {r.get('conclusion')} ({r.get('url', 'no url')})")
    pending += [f"{w} has no run yet" for w in sorted(required - seen)]
    return failed, pending


def main(argv: list[str]) -> int:
    if len(argv) != 1 or len(argv[0]) != 40:
        print("usage: main_ci.py <full commit sha>", file=sys.stderr)
        return 2
    sha, required = argv[0], push_workflows(ROOT)
    deadline = time.monotonic() + DEADLINE
    while True:
        r = subprocess.run(["gh", "run", "list", "--commit", sha, "--event", "push", "--limit", "100", "--json",
                            "databaseId,workflowName,status,conclusion,url"], capture_output=True, text=True,
                           check=False)
        if r.returncode:
            print(f"main_ci: gh run list failed: {r.stderr.strip()}", file=sys.stderr)
            return 1
        failed, pending = verdict(json.loads(r.stdout), required)
        if failed:
            print(f"main's CI on {sha[:7]} did not pass, so it does not release:\n"
                  + "\n".join(f"- {f}" for f in failed), file=sys.stderr)
            return 1
        if not pending:
            print(f"main's CI on {sha[:7]} passed: " + ", ".join(sorted(required)))
            return 0
        if time.monotonic() > deadline:
            print(f"main's CI on {sha[:7]} did not finish in {DEADLINE // 60} minutes:\n"
                  + "\n".join(f"- {p}" for p in pending), file=sys.stderr)
            return 1
        time.sleep(POLL)  # noqa: TID251 -- polling GitHub's runs, with a deadline; no event to wait on


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
