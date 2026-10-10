"""Live regression eval for the owner-request Stop hook's judge: each labelled case in owner_request_cases.json runs
the hook as the runtimes run it (JSON on stdin), against the fake bd serving that case's open needs and the real
`claude -p` Haiku judge, RUNS times (PM_LIVE_RUNS, default 3), and every run must give the case's verdict.

Skipped unless PM_LIVE_TESTS=1 and `claude` is on PATH; run with `make test-live` after editing the judge prompt.
It prints each case's pass count and the per-call latency (median, max).
"""

from __future__ import annotations

import json
import os
import shutil
import statistics
import subprocess
import tempfile
import time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

import pytest

from conftest import FAKE_BD, PM, REAL, write_config

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


def issues(case: dict) -> list[dict]:
    return [{"id": f"demo-a1.{n}", "status": "open", "issue_type": "task", **DATA["needs"][key],
             "metadata": {"session": SESSIONS[owner]}}
            for n, (key, owner) in enumerate(case["open"].items(), 1)]


def run_case(case: dict, bindir: Path, tmp: Path, root: Path) -> tuple[str, float]:
    """The hook's verdict on `case` (pass or block) and the call's seconds."""
    with tempfile.NamedTemporaryFile("w", suffix=".json", dir=tmp, delete=False) as f:
        json.dump(issues(case), f)
    # the user's HOME and Claude config, not the tests' temp ones: the real judge needs the user's claude login
    env = dict(os.environ, PATH=f"{bindir}{os.pathsep}{os.environ['PATH']}", FAKE_BD_STATE=f.name,
               FAKE_BD_LOG=os.devnull, HOME=REAL["HOME"],
               CLAUDE_CONFIG_DIR=REAL["CLAUDE_CONFIG_DIR"] or str(Path(REAL["HOME"]) / ".claude"))
    event = {"session_id": SESSIONS["me"], "hook_event_name": "Stop", "stop_hook_active": False,
             "last_assistant_message": case["reply"]}
    start = time.monotonic()
    res = subprocess.run(HOOK, input=json.dumps(event), env=env, cwd=root, capture_output=True, text=True,
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
    bindir = tmp / "bin"
    bindir.mkdir()
    (bindir / "bd").symlink_to(FAKE_BD)
    root = tmp / "repo"  # a repo pinned to this pm: the hook reads its .pm/config.toml
    subprocess.run(["git", "init", "-q", str(root)], check=True)
    write_config(root)
    jobs = [name for name in CASES for _ in range(RUNS)]
    start = time.monotonic()
    with ThreadPoolExecutor(WORKERS) as pool:
        results = list(pool.map(lambda name: run_case(CASES[name], bindir, tmp, root), jobs))
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
