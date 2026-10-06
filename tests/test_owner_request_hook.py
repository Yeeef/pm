"""The Stop hook that blocks a turn ending with an owner request asked only in chat: detection, and the hook run as
the runtimes run it (JSON on stdin) against the fake bd."""

from __future__ import annotations

import json
import os
import subprocess
import sys

import pytest

from conftest import HARNESS, fake_bd_env

HOOK = HARNESS / "owner_request_hook.py"
sys.path.insert(0, str(HARNESS))
import owner_request_hook as hook  # noqa: E402


def run(tmp_path, event, env=None, issues=None):
    env = env or fake_bd_env(tmp_path, os.environ)
    if issues is not None:
        (tmp_path / "bd.json").write_text(json.dumps(issues))
    return subprocess.run([sys.executable, str(HOOK)], input=json.dumps(event), env=env, capture_output=True,
                          text=True, timeout=10)


NEED = {"id": "demo-x1.4", "title": "Rename X?", "status": "open", "issue_type": "task", "labels": ["human"]}
CLOSED_NEED = {**NEED, "id": "demo-x1.5", "status": "closed"}
TASK = {"id": "demo-x1.6", "title": "A task", "status": "open", "issue_type": "task"}


@pytest.mark.parametrize("text", [
    "Done. needs input: should we rename X?",
    "The PR is up. Please review and merge it.",
    "Can you run `make deploy` on the server?",
    "Two options here; your call.",
    "I'm waiting on you for the API key.",
    "Should I go with option A or B?",
    "Let me know which name you prefer.",
])
def test_requests_found(text):
    assert hook.requests(text)


@pytest.mark.parametrize("text", [
    "Built the hook; tests pass (12 passed).",
    "Raised the need; nothing else waits.",
    "Is the cache warm? It is: 0.2 s.",
    "Ran it:\n```\nplease review the diff? can you run this?\n```\nAll green.",
    "The owner wrote:\n> please review the PR\nDone as asked.",
])
def test_plain_reports_pass(text):
    assert hook.requests(text) == []


def test_blocks_an_uncited_request(tmp_path):
    res = run(tmp_path, {"last_assistant_message": "needs input: should we rename X? please decide"})
    out = json.loads(res.stdout)
    assert out["decision"] == "block"
    assert all(c in out["reason"] for c in ("pm decision need --title", "pm action need --title", "pm action need --pr"))
    assert (tmp_path / "bd.log").read_text() == ""  # no citation, so bd is not asked


def test_passes_a_request_citing_an_open_need(tmp_path):
    res = run(tmp_path, {"last_assistant_message": "Raised demo-x1.4: should we rename X? Your call."},
              issues=[NEED, TASK])
    assert res.returncode == 0 and res.stdout == ""


def test_passes_a_request_citing_an_open_need_by_short_id(tmp_path):
    """The short id `pm show` prints, without the prefix, cites the need too."""
    res = run(tmp_path, {"last_assistant_message": "Raised x1.4 for this. Please merge the PR."},
              issues=[NEED, TASK])
    assert res.returncode == 0 and res.stdout == ""


def test_blocks_a_merge_request_citing_no_id(tmp_path):
    res = run(tmp_path, {"last_assistant_message": "PR #31 is ready. Please merge it."}, issues=[NEED, TASK])
    assert json.loads(res.stdout)["decision"] == "block"


@pytest.mark.parametrize("cited", ["demo-x1.5", "demo-x1.6", "demo-zz9", "x1.5", "x1.6", "zz9.1", "other-x1.4"])
def test_blocks_a_request_citing_no_open_need(tmp_path, cited):
    """A closed need, a task that is not a need, or an unknown id does not cover the request."""
    res = run(tmp_path, {"last_assistant_message": f"See {cited}. Please review it."},
              issues=[NEED, CLOSED_NEED, TASK])
    assert json.loads(res.stdout)["decision"] == "block"


def test_passes_when_stop_hook_active(tmp_path):
    res = run(tmp_path, {"stop_hook_active": True, "last_assistant_message": "Please merge the PR."})
    assert res.returncode == 0 and res.stdout == ""


def test_passes_a_plain_report(tmp_path):
    res = run(tmp_path, {"last_assistant_message": "Done; 12 tests pass."})
    assert res.returncode == 0 and res.stdout == "" and res.stderr == ""


def test_reads_the_transcript_without_last_message(tmp_path):
    lines = [{"type": "user", "message": {"role": "user", "content": "do it"}},
             {"type": "assistant", "message": {"content": [{"type": "text", "text": "Working."},
                                                           {"type": "tool_use", "name": "Bash", "input": {}}]}},
             {"type": "user", "message": {"content": [{"type": "tool_result", "content": "ok"}]}},
             {"type": "assistant", "message": {"content": [{"type": "text", "text": "Done. Can you run the deploy?"}]}},
             {"type": "system", "content": "stop hook"}]
    transcript = tmp_path / "t.jsonl"
    transcript.write_text("\n".join(json.dumps(line) for line in lines))
    assert hook.last_reply({"transcript_path": str(transcript)}) == "Done. Can you run the deploy?"
    assert json.loads(run(tmp_path, {"transcript_path": str(transcript)}).stdout)["decision"] == "block"


def test_fails_open_without_reply_text(tmp_path):
    res = run(tmp_path, {"transcript_path": str(tmp_path / "missing.jsonl")})
    assert res.returncode == 0 and res.stdout == ""
    assert "letting the stop through" in res.stderr


def test_fails_open_on_bad_input(tmp_path):
    res = subprocess.run([sys.executable, str(HOOK)], input="not json", capture_output=True, text=True)
    assert res.returncode == 0 and res.stdout == "" and "not JSON" in res.stderr


def test_fails_open_when_bd_fails(tmp_path):
    env = dict(fake_bd_env(tmp_path, os.environ), FAKE_BD_FAIL=json.dumps(["list"]))
    res = run(tmp_path, {"last_assistant_message": "Raised demo-x1.4. Please review it."}, env=env)
    assert res.returncode == 0 and res.stdout == ""
    assert "bd is unavailable" in res.stderr


def test_fails_open_without_bd(tmp_path):
    env = dict(os.environ, PATH=str(tmp_path))  # no bd anywhere on PATH
    res = run(tmp_path, {"last_assistant_message": "Raised demo-x1.4. Please review it."}, env=env)
    assert res.returncode == 0 and res.stdout == "" and "bd is unavailable" in res.stderr
