"""Parity layer 2: each test that passes on Go pm must leave the same transcript as on Python pm.

Run after the shared suite ran on Go (PM_IMPL=go), from the repository root:

    uv run --project pm python pm/tests/compare_transcripts.py

It takes the Go transcripts under $PM_TRANSCRIPTS/go (default pm/.transcripts/go) of the tests not on
go-expected-failures.txt, which the strict Go run has shown to pass, runs those tests on Python pm, and compares
each pair. It prints each differing test with a unified diff and exits 1 on any difference."""

from __future__ import annotations

import difflib
import json
import os
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

from conftest import go_expected_failures  # noqa: E402

ROOT = Path(os.environ.get("PM_TRANSCRIPTS") or HERE.parent / ".transcripts")


def main() -> int:
    expected = go_expected_failures()
    go = {}
    for path in sorted((ROOT / "go").rglob("*.json")):
        nodeid = json.loads(path.read_text())["test"]
        file, _, test = nodeid.partition("::")
        if f"{Path(file).name}::{test}" not in expected:
            go[f"{HERE / Path(file).name}::{test}"] = path
    if not go:
        print(f"no Go transcripts of passing tests under {ROOT / 'go'}; run the suite with PM_IMPL=go first")
        return 1
    env = {k: v for k, v in os.environ.items() if k != "PM_GO_BIN"} | {"PM_IMPL": "python"}
    res = subprocess.run([sys.executable, "-m", "pytest", "-q", "-p", "no:cacheprovider", "-n", "auto", *go], env=env)
    if res.returncode != 0:
        print("the tests that pass on Go failed on Python pm")
        return 1
    differ = 0
    for nodeid, path in go.items():
        py = ROOT / "python" / path.relative_to(ROOT / "go")
        if not py.is_file():  # passed on Python without a transcript: skipped there, a test for Go only
            continue
        a = py.read_text().splitlines()
        b = path.read_text().splitlines()
        if a != b:
            differ += 1
            print(f"{nodeid}: transcripts differ")
            print("\n".join(difflib.unified_diff(a, b, "python", "go", lineterm="")))
    print(f"{len(go)} transcripts of tests passing on Go compared with Python's: {differ} differ")
    return 1 if differ else 0


if __name__ == "__main__":
    sys.exit(main())
