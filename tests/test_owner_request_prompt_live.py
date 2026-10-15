"""Live regression eval for the owner-request Stop hook's judge, on each runtime it runs on: each labelled case in
owner_request_cases.json runs the hook as the runtimes run it (JSON on stdin), in a repo whose work store holds that
case's open needs, against the real judge (`--runtime claude`: Claude Haiku through `claude -p`; `--runtime codex`: a
small Codex model through `codex exec`), RUNS times (PM_LIVE_RUNS, default 3), and every run must give the case's
verdict on every runtime.

Skipped unless PM_LIVE_TESTS=1; a runtime whose CLI is not on PATH is skipped. Run with `make test-live` after editing
the judge prompt; PM_LIVE_RUNTIMES (default "claude codex") picks the runtimes. Calls run 8 at once. It prints each
runtime's pass counts and its per-call latency (median, p90, max). Claude runs as the user (their HOME and Claude
config, REAL in conftest, where its login is); Codex with a temp CODEX_HOME holding copies of the user's auth.json and
global AGENTS.md, which the real judge also reads (each mode 600, removed once the runs end).

A case is a final reply, the open needs the work store holds and the verdict the rule gives. Its label comes from the
rule, never from what the judge answers: a miss is a prompt change, not a relabel. The rule is one rule in three
texts (the autopilot request rule prompts/request_rule_autopilot.txt, prompts/owner_request_system.txt, and the block
reasons owner_request_reason.txt and owner_request_needless.txt); a rule change edits all of them in one commit and
adds a labelled case for each case it moves.
"""

from __future__ import annotations

import contextlib
import json
import os
import secrets
import shutil
import statistics
import subprocess
import time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

import pytest

from agents import login_copy
from conftest import ISSUES, PM, REAL, Repo, make_repo

pytestmark = pytest.mark.skipif(os.environ.get("PM_LIVE_TESTS") != "1",
                                reason="live model tests: set PM_LIVE_TESTS=1 (make test-live)")

HOOK = [*PM, "hook", "owner-request"]
DATA = json.loads((Path(__file__).resolve().parent / "owner_request_cases.json").read_text())
CASES = {c["name"]: c for c in DATA["cases"]}
RUNS = int(os.environ.get("PM_LIVE_RUNS", "3"))
WORKERS = 8  # concurrent judge calls; more makes each call slower
SESSIONS = {"me": "11111111-live-me", "other": "22222222-live-other"}
RUNTIMES = os.environ.get("PM_LIVE_RUNTIMES", "claude codex").split()
REAL_CODEX = Path(REAL["CODEX_HOME"] or Path(REAL["HOME"]) / ".codex")


AT = "2026-10-01T12:00:00Z"


def judge_env(runtime: str, tmp: Path, logins: contextlib.ExitStack) -> dict[str, str]:
    """The environment the real judge runs with: Claude as the user, who is logged in under their own HOME and Claude
    Code config, not the tests' temp ones; Codex with a temp CODEX_HOME holding copies of the user's login and global
    instructions (each a login_copy in `logins`, removed when it closes), so no run touches the user's Codex state."""
    env = dict(os.environ, HOME=REAL["HOME"],
               CLAUDE_CONFIG_DIR=REAL["CLAUDE_CONFIG_DIR"] or str(Path(REAL["HOME"]) / ".claude"))
    if runtime == "codex":
        home = tmp / "codex-home"
        home.mkdir()
        assert (REAL_CODEX / "auth.json").is_file(), f"Codex is not logged in: no {REAL_CODEX / 'auth.json'}"
        for name in ("auth.json", "AGENTS.md"):
            if (REAL_CODEX / name).is_file():
                logins.enter_context(login_copy(REAL_CODEX / name, home / name))
        env["CODEX_HOME"] = str(home)
    return env


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


def tool_transcript(case: dict, runtime: str, root: Path) -> str:
    """A transcript of the stopping turn in `runtime`'s own format, holding the case's ask tool calls: Claude Code's
    AskUserQuestion tool_use, Codex's request_user_input_async function_call (as Codex 0.154 records it)."""
    if runtime == "claude":
        lines = [{"type": "user", "message": {"role": "user", "content": "Go on with the doc."}},
                 *({"type": "assistant", "message": {"role": "assistant", "content": [
                     {"type": "tool_use", "id": f"t{i}", "name": "AskUserQuestion", "input": {"questions": [
                         {"question": q, "header": "Choice", "options": [{"label": "A"}, {"label": "B"}],
                          "multiSelect": False}]}}]}} for i, q in enumerate(case["tool_asks"]))]
    else:
        lines = [{"type": "turn_context", "payload": {"cwd": str(root)}},
                 *({"type": "response_item", "payload": {"type": "function_call", "name": "request_user_input_async",
                                                         "call_id": f"c{i}", "arguments": json.dumps(
                                                             {"questions": [{"title": q, "options": ["A", "B"]}]})}}
                   for i, q in enumerate(case["tool_asks"]))]
    path = root.parent / f"transcript-{runtime}-{secrets.token_hex(4)}.jsonl"  # one per run: the runs go at once
    path.write_text("".join(json.dumps(line) + "\n" for line in lines))
    return str(path)


def run_case(case: dict, root: Path, runtime: str, env: dict) -> tuple[str, float]:
    """The hook's verdict on `case` (pass, block, needless or needless+block) with its judge on `runtime`, and the
    call's seconds; "error: <stderr>" when the check did not run."""
    event = {"session_id": SESSIONS["me"], "hook_event_name": "Stop", "stop_hook_active": False,
             "last_assistant_message": case["reply"],
             **({"transcript_path": tool_transcript(case, runtime, root)} if case.get("tool_asks") else {})}
    start = time.monotonic()
    res = subprocess.run([*HOOK, "--runtime", runtime], input=json.dumps(event), cwd=root, env=env,
                         capture_output=True, text=True, timeout=60, check=False)
    took = time.monotonic() - start
    if res.returncode != 0:
        return f"error: {res.stderr.strip()}", took
    if not res.stdout.strip():
        return "pass", took
    reason = json.loads(res.stdout)["reason"]
    needless, request = "authorized to take without asking" in reason, "no open request of this session" in reason
    return {(True, False): "needless", (False, True): "block", (True, True): "needless+block"}[needless, request], took


@pytest.fixture(scope="module", params=RUNTIMES)
def verdicts(request, tmp_path_factory):
    """Every case's verdicts, RUNS each, on one runtime's judge."""
    runtime = request.param
    if shutil.which(runtime) is None:
        pytest.skip(f"{runtime} is not on PATH")
    tmp = tmp_path_factory.mktemp(f"live-{runtime}")
    with contextlib.ExitStack() as logins:
        env = judge_env(runtime, tmp, logins)
        with ThreadPoolExecutor(WORKERS) as pool:
            clones = dict(zip(CASES, pool.map(lambda nc: clone(nc[0], CASES[nc[1]], tmp), enumerate(CASES)),
                              strict=True))
        try:
            jobs = [name for name in CASES for _ in range(RUNS)]
            start = time.monotonic()
            with ThreadPoolExecutor(WORKERS) as pool:
                results = list(pool.map(lambda name: run_case(CASES[name], clones[name].root, runtime, env), jobs))
        finally:
            for r in clones.values():
                r.stop_service()
    times = [t for _, t in results]
    out = {name: [v for n, (v, _) in zip(jobs, results, strict=True) if n == name] for name in CASES}
    passed = sum(v == CASES[n]["expect"] for n, (v, _) in zip(jobs, results, strict=True))
    print(f"\n{runtime}: {passed}/{len(jobs)} hook runs gave the case's verdict, in {time.monotonic() - start:.1f}s, "
          f"{WORKERS} at a time; per run median {statistics.median(times):.2f}s, "
          f"p90 {statistics.quantiles(times, n=10)[-1]:.2f}s, max {max(times):.2f}s")
    for name, got in out.items():
        want = CASES[name]["expect"]
        print(f"  {sum(v == want for v in got)}/{RUNS}  {want:5}  {name}"
              + "".join(f"\n      got {v}" for v in sorted(set(got)) if v != want))
    return out


@pytest.mark.parametrize("name", CASES)
def test_owner_request_judge(verdicts, name):
    want = CASES[name]["expect"]
    assert verdicts[name] == [want] * RUNS, f"{name}: got {verdicts[name]}, want {want} in all {RUNS} runs"
