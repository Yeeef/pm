"""The Stop hook that blocks a turn ending with an owner request asked only in chat, run as the runtimes run it (JSON
on stdin) against the fake bd and a fake `claude` judge: which needs it reads, how it calls the judge, how it turns
the judge's answer into a verdict, and how it fails. The judge's own accuracy is the live eval's job
(test_owner_request_prompt_live.py)."""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
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


def test_reads_only_this_sessions_open_needs(tmp_path):
    """The judge sees this session's open human issues and nothing else: not another session's, a closed one, one
    with no session, or a task that is not a need."""
    res = run(tmp_path, judged=answer())
    assert res.returncode == 0 and res.stdout == "", res.stderr
    assert bd_calls(tmp_path) == [["list", "--label", "human", "--metadata-field", f"session={ME}", "--limit", "0",
                                   "--json"]]
    prompt = claude_calls(tmp_path)[0]["stdin"]
    assert "demo-x1.4: Rename X?" in prompt and "Options: X, Y. Default: X." in prompt
    for other in (THEIRS, CLOSED, NO_SESSION, TASK):
        assert other["id"] not in prompt and other["title"] not in prompt
    assert "Should we rename X?" in prompt


def test_judge_runs_haiku_without_thinking_settings_or_tools(tmp_path):
    run(tmp_path, judged=answer())
    call = claude_calls(tmp_path)[0]
    args = call["args"]
    assert args[:2] == ["-p", "--model"] and args[2] == "claude-haiku-4-5-20251001"
    for flag, value in (("--setting-sources", ""), ("--tools", ""), ("--output-format", "text")):
        assert args[args.index(flag) + 1] == value
    assert {"--strict-mcp-config", "--no-session-persistence", "--system-prompt"} <= set(args)
    assert call["env"] == {"MAX_THINKING_TOKENS": "0", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"}
    assert not Path(call["cwd"]).resolve().is_relative_to(ROOT.resolve())  # no project CLAUDE.md


def test_lists_none_when_the_session_has_no_open_need(tmp_path):
    run(tmp_path, judged=answer(), issues=[THEIRS, CLOSED])
    assert "OPEN REQUESTS:\n(none)" in claude_calls(tmp_path)[0]["stdin"]


def test_passes_a_request_matching_this_sessions_need(tmp_path):
    res = run(tmp_path, judged=answer(("Should we rename X?", "decision", MINE["id"])))
    assert res.returncode == 0 and res.stdout == "", res.stderr


@pytest.mark.parametrize("match", [None, THEIRS["id"], CLOSED["id"], TASK["id"], "demo-zz9"])
def test_blocks_a_request_matching_no_open_need_of_this_session(tmp_path, match):
    """A match the judge names counts only when it is one of this session's open needs, so another session's need,
    a closed one, a task or an invented id does not cover the request."""
    res = run(tmp_path, reply="Should I merge the PR now or wait for review?",
              judged=answer(("Should I merge the PR now or wait for review?", "decision", match)))
    out = json.loads(res.stdout)
    assert res.returncode == 0 and out["decision"] == "block"
    assert '"Should I merge the PR now or wait for review?"' in out["reason"]
    assert all(c in out["reason"] for c in ("pm decision need --title", "pm action need --title",
                                            "pm action need --pr"))
    assert "needs no id" in out["reason"] and "cite" not in out["reason"]


def test_blocks_when_one_of_two_requests_is_unmatched(tmp_path):
    res = run(tmp_path, judged=answer(("Rename X?", "decision", MINE["id"]), ("Please merge PR #9.", "review", None)))
    reason = json.loads(res.stdout)["reason"]
    assert '"Please merge PR #9."' in reason and "Rename X?" not in reason


def test_blocks_a_needless_ask_even_when_an_open_need_matches(tmp_path):
    """Leave to push the branch or open the PR is never needed: it blocks with its own reason, matched or not."""
    res = run(tmp_path, reply="Should I push the branch and open the PR?",
              judged=answer(("Should I push the branch and open the PR?", "authorized", MINE["id"])))
    reason = json.loads(res.stdout)["reason"]
    assert reason.startswith("Your reply asks the owner's leave for, or offers, a step you are authorized")
    assert '"Should I push the branch and open the PR?"' in reason and "Do the step now" in reason
    assert "pm decision need" not in reason


def test_gives_both_reasons_for_a_needless_ask_and_an_unmatched_request(tmp_path):
    res = run(tmp_path, judged=answer(("Shall I open the PR?", "authorized", None), ("Merge it?", "review", None)))
    reason = json.loads(res.stdout)["reason"]
    assert "Do the step now" in reason and "pm action need --pr" in reason and "pm decision need" in reason


@pytest.mark.parametrize("kind", ["clarification", "offer", "suggestion", "not asked"])
def test_passes_sentences_that_ask_nothing_sprint_work_waits_on(tmp_path, kind):
    res = run(tmp_path, judged=answer(("If you want a live check too, tell me.", kind, None)))
    assert res.returncode == 0 and res.stdout == "", res.stderr


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


def test_fails_when_bd_fails(tmp_path):
    env = dict(fake_bd_env(tmp_path, os.environ), FAKE_BD_FAIL=json.dumps(["list"]))
    failed(run(tmp_path, env=env), "bd list failed (exit 1)")
    assert claude_calls(tmp_path) == []


def only_git(tmp_path):
    """A PATH dir holding only git, which pm needs to find its config."""
    d = tmp_path / "git-only"
    d.mkdir()
    (d / "git").symlink_to(shutil.which("git"))
    return d


def test_fails_without_bd(tmp_path):
    env = dict(fake_bd_env(tmp_path, os.environ), PATH=str(only_git(tmp_path)))
    failed(run(tmp_path, env=env), "bd is not installed")


def test_fails_when_claude_fails(tmp_path):
    env = dict(fake_bd_env(tmp_path, os.environ), FAKE_CLAUDE_FAIL="Not logged in")
    failed(run(tmp_path, env=env), "claude -p failed (exit 1): Not logged in")


def test_fails_without_claude(tmp_path):
    env = fake_bd_env(tmp_path, os.environ)
    (tmp_path / "bin" / "claude").unlink()
    # the fake bd needs python3
    path = os.pathsep.join(map(str, (tmp_path / "bin", Path(sys.executable).parent, only_git(tmp_path))))
    failed(run(tmp_path, env=dict(env, PATH=path)), "claude is not installed")


@pytest.mark.parametrize("judged", ["Summary 1.", '{"items": "none"}', '{"items": [{"quote": "x"}]}', "{oops}",
                                    '{"items": [{"quote": "x", "kind": "decision", "match": [1]}]}'])
def test_fails_on_a_verdict_out_of_shape(tmp_path, judged):
    failed(run(tmp_path, judged=judged), "claude -p answered out of shape")


@pytest.mark.parametrize("event, said", [({"session_id": None}, "no session_id"),
                                         ({"last_assistant_message": None}, "no last_assistant_message")])
def test_fails_on_input_missing_a_field(tmp_path, event, said):
    failed(run(tmp_path, **event), said)


def test_fails_on_input_that_is_not_json(tmp_path):
    res = subprocess.run(HOOK, input="not json", cwd=ROOT, capture_output=True, text=True)
    failed(res, "not a JSON object")
