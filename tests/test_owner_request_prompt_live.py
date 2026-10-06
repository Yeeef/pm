"""Live regression tests for the Stop hook's Haiku owner-request prompt in `.claude/settings.json`.

Each case fills `$ARGUMENTS` with a synthetic Stop hook input and asks the hook's own model through `claude -p`,
RUNS times, and requires every run to agree. Skipped unless PM_LIVE_TESTS=1 and `claude` is on PATH; run them
with `make test-live` after editing the prompt. This approximates the runtime's prompt-hook call (same prompt and
model, a minimal system prompt of ours), not the call itself.
"""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import tempfile
import time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

import pytest

pytestmark = pytest.mark.skipif(
    os.environ.get("PM_LIVE_TESTS") != "1" or shutil.which("claude") is None,
    reason="live model tests: set PM_LIVE_TESTS=1 (make test-live) with `claude` on PATH",
)

ROOT = Path(__file__).resolve().parents[4]
RUNS = 3
SYSTEM = "You evaluate a hook condition. Answer with the JSON object only."

CASES = {
    "short id decision": ("Please decide 9va.15.3: should sprint 15 ship the renderer change now or after the "
                          "records migration? Default is after.", True),
    "full id decision": ("Please decide yeeef-agents-9va.15.3: should sprint 15 ship the renderer change now or "
                         "after the records migration? Default is after.", True),
    "offer": ("Done: the render check passes. If you want a Codex live check too, tell me and I will run one.",
              True),
    "clarification": ("Before I start: by 'the hook' in your message, do you mean the Stop hook or the "
                      "SessionStart hook?", True),
    "status report": ("Sprint 26 status: the hook accepts short ids, the tests pass, and the PR is open. "
                      "Next I will write the delivery report.", True),
    "PR merge without id": ("PR #99 is ready; please review and merge it.", False),
    "decision without id": ("Should sprint 15 use option A or B? Please decide.", False),
}


def hook_prompt() -> tuple[str, str]:
    settings = json.loads((ROOT / ".claude" / "settings.json").read_text())
    found = [h for entry in settings["hooks"]["Stop"] for h in entry["hooks"] if h.get("type") == "prompt"]
    assert len(found) == 1, f"expected one Stop prompt hook, found {len(found)}"
    return found[0]["prompt"], found[0]["model"]


def judge(prompt: str, model: str, reply: str) -> bool:
    hook_input = {"session_id": "live-test", "hook_event_name": "Stop", "stop_hook_active": False,
                  "last_assistant_message": reply}
    res = subprocess.run(
        ["claude", "-p", "--model", model, "--system-prompt", SYSTEM, "--tools", "", "--setting-sources", "",
         "--strict-mcp-config", "--disable-slash-commands", "--no-session-persistence", "--output-format", "text"],
        input=prompt.replace("$ARGUMENTS", json.dumps(hook_input)), capture_output=True, text=True, timeout=90,
        cwd=tempfile.gettempdir(),
    )
    assert res.returncode == 0, res.stderr
    match = re.search(r"\{.*\}", res.stdout, re.S)
    assert match, f"no JSON in answer: {res.stdout!r}"
    return json.loads(match.group(0))["ok"]


@pytest.fixture(scope="module")
def verdicts():
    prompt, model = hook_prompt()
    jobs = [(name, reply) for name, (reply, _) in CASES.items() for _ in range(RUNS)]
    start = time.monotonic()
    with ThreadPoolExecutor(len(jobs)) as pool:
        results = list(pool.map(lambda job: judge(prompt, model, job[1]), jobs))
    out = {name: [ok for (n, _), ok in zip(jobs, results) if n == name] for name in CASES}
    print(f"\n{model}, {len(jobs)} calls in {time.monotonic() - start:.1f}s")
    for name, oks in out.items():
        want = CASES[name][1]
        print(f"  {name}: expected ok={want}, matched {sum(ok == want for ok in oks)}/{RUNS}")
    return out


@pytest.mark.parametrize("name", CASES)
def test_owner_request_prompt(verdicts, name):
    want = CASES[name][1]
    assert verdicts[name] == [want] * RUNS, f"{name}: got {verdicts[name]}, want ok={want} in all {RUNS} runs"
