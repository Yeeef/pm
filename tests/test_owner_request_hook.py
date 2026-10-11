"""The Stop hook that blocks a turn ending with an owner request asked only in chat, run as the runtimes run it (JSON
on stdin) in a temp repo whose seeds hold this session's needs and others, against a fake `claude` judge: how it turns
the judge's answer into a verdict, and that it fails hard. The judge's own accuracy is the live eval's job
(test_owner_request_prompt_live.py)."""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from conftest import hook_payload

ME, OTHER = "sess-me", "sess-other"
AT = "2026-10-01T12:00:00Z"

MINE = {"id": "repo-demo.1.4", "title": "Rename X?", "description": "Options: X, Y. Default: X.", "status": "open",
        "issue_type": "task", "parent": "repo-demo.1", "labels": ["human"], "metadata": {"session": ME},
        "created_at": AT, "updated_at": AT}
THEIRS = {**MINE, "id": "repo-demo.1.5", "title": "Merge PR #7?", "metadata": {"session": OTHER}}
CLOSED = {**MINE, "id": "repo-demo.1.6", "title": "Old question?", "status": "closed", "closed_at": AT}
NO_SESSION = {**MINE, "id": "repo-demo.1.7", "title": "Raised outside a session?", "metadata": {}}
# a task this session holds: work, not a request to the owner
TASK = {"id": "repo-demo.1.8", "title": "A task", "status": "in_progress", "issue_type": "task", "parent": "repo-demo.1",
        "metadata": {"claimed_by": ME, "claimed_at": AT}, "created_at": AT, "updated_at": AT, "started_at": AT}
ISSUES = [MINE, THEIRS, CLOSED, NO_SESSION, TASK]


@pytest.fixture
def needs(repo):
    """The repo with this session's open need, another session's, this session's closed one, one no session raised
    and a task this session holds."""
    for issue in ISSUES:
        repo.add_issue(issue)
    return repo


def answer(*items):
    return json.dumps({"items": [{"quote": q, "kind": k, "match": m} for q, k, m in items]})


def run(repo, reply="Should we rename X?", judged=None, fail=None, **event):
    env = repo.env
    repo.env = dict(env, **({"FAKE_CLAUDE_OUTPUT": judged} if judged is not None else {}),
                    **({"FAKE_CLAUDE_FAIL": fail} if fail else {}))
    event = {"session_id": ME, "hook_event_name": "Stop", "stop_hook_active": False,
             "last_assistant_message": reply, **event}
    try:
        return repo.pm("hook", "owner-request", stdin=json.dumps(event))
    finally:
        repo.env = env


def claude_calls(repo):
    log = Path(repo.env["FAKE_CLAUDE_LOG"])
    return [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []


def test_blocks_a_needless_ask_even_when_an_open_need_matches(needs):
    """Leave to push the branch or open the PR is never needed: it blocks with its own reason, matched or not."""
    # folded: test_passes_a_request_matching_this_sessions_need
    res = run(needs, judged=answer(("Should we rename X?", "decision", MINE["id"])))
    assert res.returncode == 0 and res.stdout == "", res.stderr
    (call,) = claude_calls(needs)
    assert call["stdin"] == ("OPEN REQUESTS:\n- repo-demo.1.4: Rename X?\n  Options: X, Y. Default: X.\n\n"
                             "REPLY:\n<<<\nShould we rename X?\n>>>"), "only this session's open needs are listed"
    assert call["args"][:2] == ["-p", "--model"] and call["args"][-2] == "--system-prompt"
    assert call["env"] == {"MAX_THINKING_TOKENS": "0", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"}

    res = run(needs, reply="Should I push the branch and open the PR?",
              judged=answer(("Should I push the branch and open the PR?", "authorized", MINE["id"])))
    reason = json.loads(res.stdout)["reason"]
    assert reason.startswith("Your reply asks the owner's leave for, or offers, a step you are authorized")
    assert '"Should I push the branch and open the PR?"' in reason and "Do the step now" in reason
    assert "pm decision need" not in reason

    # folded: test_blocks_a_request_matching_no_open_need_of_this_session
    # A match the judge names counts only when it is one of this session's open needs, so another session's need
    # does not cover the request.
    for match in (None, THEIRS["id"]):
        res = run(needs, reply="Should I merge the PR now or wait for review?",
                  judged=answer(("Should I merge the PR now or wait for review?", "decision", match)))
        out = json.loads(res.stdout)
        assert res.returncode == 0 and out["decision"] == "block", match
        assert '"Should I merge the PR now or wait for review?"' in out["reason"]
        assert all(c in out["reason"] for c in ("pm decision need --title", "pm action need --title",
                                                "pm action need --pr"))
        assert "needs no id" in out["reason"] and "cite" not in out["reason"]

    # folded: test_blocks_a_clarification_asked_only_in_chat
    # A clarification is a request like a decision: asked in plain text, with no open need, it blocks.
    q = "By 'the hook', do you mean the Stop hook or the SessionStart hook?"
    res = run(needs, reply=q, judged=answer((q, "clarification", None)))
    out = json.loads(res.stdout)
    assert res.returncode == 0 and out["decision"] == "block"
    assert f'"{q}"' in out["reason"] and "AskUserQuestion" in out["reason"]


def failed(res, said):
    """The check could not run: exit 1 (shown, never blocking), the cause on stderr, no verdict."""
    assert res.returncode == 1 and res.stdout == "" and said in res.stderr, res.stderr
    assert "the owner-request check did not run" in res.stderr


def test_fails_when_claude_fails(needs):
    # folded: test_passes_without_reading_anything
    # After one block (stop_hook_active) it never loops, and an empty reply asks nothing: blank, null (Codex's turn
    # that ended without a message) or missing.
    for event in ({"stop_hook_active": True}, {"last_assistant_message": "  \n"}, {"last_assistant_message": None}):
        res = run(needs, **event)
        assert res.returncode == 0 and res.stdout == "" and res.stderr == ""
        assert claude_calls(needs) == []
    res = needs.pm("hook", "owner-request", stdin=json.dumps({"session_id": ME, "hook_event_name": "Stop"}))
    assert res.returncode == 0 and res.stdout == "" and res.stderr == "" and claude_calls(needs) == []

    failed(run(needs, fail="Not logged in"), "claude -p failed (exit 1): Not logged in")


@pytest.mark.parametrize("asked", [True, False], ids=["asked-only-in-chat", "asked-as-a-need"])
def test_claude_codes_and_codexs_stop_input_get_the_same_verdict(needs, asked):
    """The same reply, asking what an open need of this session holds or what none does, in Claude Code's and Codex's
    real Stop input: the same verdict."""
    reply = "Should we rename X?"
    judged = answer((reply, "decision", None if asked else MINE["id"]))
    got = {}
    for runtime in ("claude", "codex"):
        event = hook_payload(runtime, "Stop", session=ME, cwd=str(needs.root), transcript=str(needs.root / "t.jsonl"),
                             reply=reply)
        res = run(needs, judged=judged, **event)
        got[runtime] = (res.returncode, res.stdout)
    assert got["claude"] == got["codex"]
    assert got["codex"][0] == 0
    assert json.loads(got["codex"][1])["decision"] == "block" if asked else got["codex"][1] == ""


def test_passes_codexs_stop_without_a_message(needs):
    """Codex sends last_assistant_message null for a turn that ended without one: nothing was asked."""
    res = run(needs, **hook_payload("codex", "Stop", session=ME, cwd=str(needs.root),
                                    transcript=str(needs.root / "t.jsonl"), reply=None))
    assert res.returncode == 0 and res.stdout == "" and res.stderr == ""
    assert claude_calls(needs) == []
