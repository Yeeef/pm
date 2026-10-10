"""Live regression eval for the owner-request Stop hook's judge: each labelled case in owner_request_cases.json runs
the hook as the runtimes run it (JSON on stdin), in a repo whose work store holds that case's open needs, against the
real `claude -p` Haiku judge, RUNS times (PM_LIVE_RUNS, default 3), and every run must give the case's verdict.

Skipped unless PM_LIVE_TESTS=1 and `claude` is on PATH; run with `make test-live` after editing the judge prompt.
It prints each case's pass count and the per-call latency (median, max).
"""

from __future__ import annotations

import json
import os
import shutil
import statistics
import subprocess
import time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

import pytest

from conftest import ISSUES, PM, REAL, Repo, make_repo

pytestmark = pytest.mark.skipif(
    os.environ.get("PM_LIVE_TESTS") != "1" or shutil.which("claude") is None,
    reason="live model tests: set PM_LIVE_TESTS=1 (make test-live) with `claude` on PATH",
)

HOOK = [*PM, "hook", "owner-request"]
DATA = json.loads((Path(__file__).resolve().parent / "owner_request_cases.json").read_text())
CASES = {c["name"]: c for c in DATA["cases"]}
RUNS = int(os.environ.get("PM_LIVE_RUNS", "3"))
WORKERS = 8  # concurrent claude calls; more makes each call slower
SESSIONS = {"me": "11111111-live-me", "other": "22222222-live-other"}


AT = "2026-10-01T12:00:00Z"
# the real claude runs as the user, who is logged in under their own HOME and Claude Code config, not the tests' temp ones
CLAUDE_ENV = dict(os.environ, HOME=REAL["HOME"],
                  CLAUDE_CONFIG_DIR=REAL["CLAUDE_CONFIG_DIR"] or str(Path(REAL["HOME"]) / ".claude"))


def issues(case: dict) -> list[dict]:
    """The fixture's seeds and the case's open needs, each raised by its session, as bd exports them."""
    return [*ISSUES, *({"id": f"repo-demo.1.{n}", "status": "open", "issue_type": "task", "parent": "repo-demo.1",
                        **DATA["needs"][key], "metadata": {"session": SESSIONS[owner]}, "created_at": AT,
                        "updated_at": AT}
                       for n, (key, owner) in enumerate(case["open"].items(), 3))]


def clone(n: int, case: dict, tmp: Path) -> Repo:
    """A repo pinned to this pm whose work store holds the case's needs, held by its pm service; under a short path,
    which the service's socket needs."""
    (tmp / str(n)).mkdir()
    return make_repo(tmp / str(n), issues(case))


def run_case(case: dict, root: Path) -> tuple[str, float]:
    """The hook's verdict on `case` (pass or block) and the call's seconds."""
    event = {"session_id": SESSIONS["me"], "hook_event_name": "Stop", "stop_hook_active": False,
             "last_assistant_message": case["reply"]}
    start = time.monotonic()
    res = subprocess.run(HOOK, input=json.dumps(event), cwd=root, env=CLAUDE_ENV, capture_output=True, text=True,
                         timeout=60)
    took = time.monotonic() - start
    assert res.returncode == 0, res.stderr
    if not res.stdout.strip():
        return "pass", took
    reason = json.loads(res.stdout)["reason"]
    needless, request = "authorized to take without asking" in reason, "no open request of this session" in reason
    return {(True, False): "needless", (False, True): "block", (True, True): "needless+block"}[needless, request], took


@pytest.fixture(scope="module")
def verdicts(tmp_path_factory):
    tmp = tmp_path_factory.mktemp("live")
    with ThreadPoolExecutor(WORKERS) as pool:
        clones = dict(zip(CASES, pool.map(lambda nc: clone(nc[0], CASES[nc[1]], tmp), enumerate(CASES))))
    try:
        jobs = [name for name in CASES for _ in range(RUNS)]
        start = time.monotonic()
        with ThreadPoolExecutor(WORKERS) as pool:
            results = list(pool.map(lambda name: run_case(CASES[name], clones[name].root), jobs))
    finally:
        for r in clones.values():
            r.stop_service()
    times = [t for _, t in results]
    out = {name: [v for n, (v, _) in zip(jobs, results) if n == name] for name in CASES}
    print(f"\n{len(jobs)} hook runs in {time.monotonic() - start:.1f}s, {WORKERS} at a time; per run median "
          f"{statistics.median(times):.2f}s, max {max(times):.2f}s")
    for name, got in out.items():
        want = CASES[name]["expect"]
        print(f"  {sum(v == want for v in got)}/{RUNS}  {want:5}  {name}")
    return out


@pytest.mark.parametrize("name", CASES)
def test_owner_request_judge(verdicts, name):
    want = CASES[name]["expect"]
    assert verdicts[name] == [want] * RUNS, f"{name}: got {verdicts[name]}, want {want} in all {RUNS} runs"
