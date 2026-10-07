"""The Stop hook that blocks a turn ending with an owner request asked only in chat, run as the runtimes run it (JSON
on stdin) against the fake bd and a fake `claude` judge: how it turns the judge's answer into a verdict, and that it
fails open. The judge's own accuracy is the live eval's job
(test_owner_request_prompt_live.py)."""

from __future__ import annotations

import json
import os
import subprocess
from pathlib import Path

import pytest

from conftest import PM, fake_bd_env

HOOK = [*PM, "hook", "owner-request"]
ROOT = Path(__file__).resolve().parents[2]  # this repo: pm needs its .pm/config.toml
ME, OTHER = "sess-me", "sess-other"

MINE = {"id": "demo-x1.4", "title": "Rename X?", "description": "Options: X, Y. Default: X.", "status": "open",
        "issue_type": "task", "labels": ["human"], "metadata": {"session": ME}}
THEIRS = {**MINE, "id": "demo-x1.5", "title": "Merge PR #7?", "metadata": {"session": OTHER}}
CLOSED = {**MINE, "id": "demo-x1.6", "title": "Old question?", "status": "closed"}
NO_SESSION = {**MINE, "id": "demo-x1.7", "title": "Raised outside a session?", "metadata": {}}
TASK = {"id": "demo-x1.8", "title": "A task", "status": "open", "issue_type": "task", "metadata": {"session": ME}}
ISSUES = [MINE, THEIRS, CLOSED, NO_SESSION, TASK]


def answer(*items):
    return json.dumps({"items": [{"quote": q, "kind": k, "match": m} for q, k, m in items]})


def run(tmp_path, reply="Should we rename X?", judged=None, env=None, issues=ISSUES, **event):
    env = env or fake_bd_env(tmp_path, os.environ)
    if judged is not None:
        env = dict(env, FAKE_CLAUDE_OUTPUT=judged)
    (tmp_path / "bd.json").write_text(json.dumps(issues))
    event = {"session_id": ME, "hook_event_name": "Stop", "stop_hook_active": False,
             "last_assistant_message": reply, **event}
    return subprocess.run(HOOK, input=json.dumps(event), env=env, cwd=ROOT, capture_output=True, text=True, timeout=20)


def bd_calls(tmp_path):
    return [json.loads(l) for l in (tmp_path / "bd.log").read_text().splitlines()]


def claude_calls(tmp_path):
    log = tmp_path / "claude.log"
    return [json.loads(l) for l in log.read_text().splitlines()] if log.exists() else []


def test_passes_a_request_matching_this_sessions_need(tmp_path):
    res = run(tmp_path, judged=answer(("Should we rename X?", "decision", MINE["id"])))
    assert res.returncode == 0 and res.stdout == "", res.stderr


@pytest.mark.parametrize("match", [None, THEIRS["id"]])
def test_blocks_a_request_matching_no_open_need_of_this_session(tmp_path, match):
    """A match the judge names counts only when it is one of this session's open needs, so another session's need
    does not cover the request."""
    res = run(tmp_path, reply="Should I merge the PR now or wait for review?",
              judged=answer(("Should I merge the PR now or wait for review?", "decision", match)))
    out = json.loads(res.stdout)
    assert res.returncode == 0 and out["decision"] == "block"
    assert '"Should I merge the PR now or wait for review?"' in out["reason"]
    assert all(c in out["reason"] for c in ("pm decision need --title", "pm action need --title",
                                            "pm action need --pr"))
    assert "needs no id" in out["reason"] and "cite" not in out["reason"]


def test_blocks_a_needless_ask_even_when_an_open_need_matches(tmp_path):
    """Leave to push the branch or open the PR is never needed: it blocks with its own reason, matched or not."""
    res = run(tmp_path, reply="Should I push the branch and open the PR?",
              judged=answer(("Should I push the branch and open the PR?", "authorized", MINE["id"])))
    reason = json.loads(res.stdout)["reason"]
    assert reason.startswith("Your reply asks the owner's leave for, or offers, a step you are authorized")
    assert '"Should I push the branch and open the PR?"' in reason and "Do the step now" in reason
    assert "pm decision need" not in reason


@pytest.mark.parametrize("event", [{"stop_hook_active": True}, {"last_assistant_message": "  \n"}])
def test_passes_without_reading_anything(tmp_path, event):
    """After one block (stop_hook_active) it never loops, and an empty reply asks nothing."""
    res = run(tmp_path, **event)
    assert res.returncode == 0 and res.stdout == "" and res.stderr == ""
    assert bd_calls(tmp_path) == [] and claude_calls(tmp_path) == []


def failed(res, said):
    """The check could not run: exit 1 (shown, never blocking), the cause on stderr, no verdict."""
    assert res.returncode == 1 and res.stdout == "" and said in res.stderr, res.stderr
    assert "the owner-request check did not run" in res.stderr


def test_fails_when_claude_fails(tmp_path):
    env = dict(fake_bd_env(tmp_path, os.environ), FAKE_CLAUDE_FAIL="Not logged in")
    failed(run(tmp_path, env=env), "claude -p failed (exit 1): Not logged in")
