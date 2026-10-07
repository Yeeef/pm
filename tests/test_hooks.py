"""`pm prime` (the session and subagent context), `pm hook stop` (blocks on this session's uncommitted records), and
the render check on generated sections. The hooks run as the runtimes run them: JSON on stdin, in a temp clone."""

from __future__ import annotations

import json
import re
import shutil
import subprocess
import sys
from pathlib import Path

from conftest import PM, write_config

from pm import hooks

SESSION = [*PM, "prime", "--hook-json"]
SUBAGENT = [*PM, "prime", "--subagent", "--hook-json"]
STOP = [*PM, "hook", "stop"]


def run(cmd, event, env, cwd):
    return subprocess.run(cmd, input=json.dumps(event), env=env, cwd=cwd, capture_output=True, text=True, timeout=60)


# ---------------------------------------------------------------- session context


def shown_part(text):
    """The `pm show` part of the session context, after the rules and the command list."""
    head = hooks.head()
    assert text.startswith(head) and head.startswith("# pm rules\n")
    return text[len(head):]


def test_session_start_injects_rules_and_pm_show(repo):
    res = run(SESSION, {"hook_event_name": "SessionStart", "cwd": str(repo.root)}, repo.env, repo.root)
    assert res.returncode == 0, res.stderr
    out = json.loads(res.stdout)["hookSpecificOutput"]
    assert out["hookEventName"] == "SessionStart"
    shown = repo.pm("show").stdout.strip()
    header, _, body = shown_part(out["additionalContext"]).partition("\n\n")
    assert re.fullmatch(r"Project state from `bin/pm show` at session start, \d{4}-\d\d-\d\d \d\d:\d\d UTC: .*"
                        r"run `bin/pm show` again before stating project state to the owner\.", header)
    assert body == shown
    assert "Sprint 1: First" in shown
    plain = repo.pm("prime")  # by hand: the same text, no envelope
    assert plain.returncode == 0 and plain.stdout.strip() == out["additionalContext"]


def test_session_start_cuts_long_output_at_a_line(tmp_path):
    long = "\n".join(f"line {n} " + "x" * 90 for n in range(200))
    text = hooks.context(str(tmp_path), [sys.executable, "-c", f"print({long!r})"])
    assert len(text) <= hooks.CAP
    assert text.endswith(hooks.CUT)
    assert text[:-len(hooks.CUT)].endswith("x" * 90)  # whole lines only


def test_session_start_fails_open_with_one_line(repo):
    (repo.state).write_text("not json")  # the fake bd now fails, so pm show fails
    res = run(SESSION, {"cwd": str(repo.root)}, repo.env, repo.root)
    assert res.returncode == 0
    text = shown_part(json.loads(res.stdout)["hookSpecificOutput"]["additionalContext"])
    assert text.startswith("pm show failed at session start (") and "\n" not in text


def test_session_context_stays_within_the_cap_with_the_rules(tmp_path, monkeypatch):
    long = "\n".join(f"line {n} " + "x" * 90 for n in range(200))
    monkeypatch.setattr(hooks, "SHOW", [sys.executable, "-c", f"print({long!r})"])
    text = hooks.prime(str(tmp_path))
    assert len(text) <= hooks.CAP and text.endswith(hooks.CUT) and shown_part(text).startswith("Project state")


def subcommands():
    from pm.cli import parser
    return next(a for a in parser()._subparsers._group_actions if a.dest == "cmd").choices


def listed_nouns():
    line = hooks.commands().split("\n")[2]
    assert line.startswith("`pm` nouns: ") and line.endswith(".")
    return re.findall(r"`(\w+)`", line[len("`pm` nouns: "):])


def test_prime_lists_every_agent_command_from_the_parser():
    listed = listed_nouns()
    assert listed == [c for c in subcommands() if c not in {"prime", "hook", "push"}]
    assert "prime" not in listed and "show" in listed
    assert hooks.commands().startswith("## Commands\n\n")
    assert hooks.commands().endswith("\nRun `pm <noun> --help` for its commands and flags.")
    assert hooks.commands().count("\n") == 3  # compact: a heading, the nouns, the help pointer


def test_prime_lists_a_new_command_without_editing_prime_md(monkeypatch):
    from pm import cli
    build = cli.parser

    def extended():
        ap = build()
        next(a for a in ap._subparsers._group_actions if a.dest == "cmd").add_parser(
            "frobnicate", help="frobnicate the records; " + "x" * 200)
        return ap
    monkeypatch.setattr(cli, "parser", extended)
    assert listed_nouns()[-1] == "frobnicate"
    assert "frobnicate" not in hooks.rules()


def test_session_start_fails_open_when_pm_is_missing(tmp_path):
    text = hooks.context(str(tmp_path), [str(tmp_path / "no-such-pm")])
    assert text.startswith("pm show did not run at session start (") and "\n" not in text


def fake(out, code=0):
    return [sys.executable, "-c", f"import sys; print({out!r}); sys.exit({code})"]


def test_subagent_start_names_the_profile(tmp_path):
    text = hooks.profile(str(tmp_path), fake('{"key": "agent.profile", "value": "team-maintainer"}'))
    assert text == "Beads agent profile: team-maintainer (commit and push are routine unless your brief says otherwise)."
    text = hooks.profile(str(tmp_path), fake('{"key": "agent.profile", "value": "conservative"}'))
    assert text == "Beads agent profile: conservative."


def test_subagent_start_says_why_when_bd_fails(tmp_path):
    for cmd in (fake("boom", 1), fake('{"value": ""}'), [str(tmp_path / "no-such-bd")]):
        text = hooks.profile(str(tmp_path), cmd)
        assert text.startswith("Beads agent profile: unknown (bd config failed: ") and "\n" not in text


def test_subagent_start_envelope(tmp_path):
    subprocess.run(["git", "init", "-q", str(tmp_path)], check=True)
    write_config(tmp_path)
    (tmp_path / "bin").mkdir()
    (tmp_path / "bin/git").symlink_to(shutil.which("git"))
    event = {"hook_event_name": "SubagentStart", "cwd": str(tmp_path)}
    res = run(SUBAGENT, event, {"PATH": str(tmp_path / "bin")}, tmp_path)  # git for pm's config check; no bd
    assert res.returncode == 0, res.stderr
    out = json.loads(res.stdout)["hookSpecificOutput"]
    assert out["hookEventName"] == "SubagentStart"
    text = out["additionalContext"]
    assert text.startswith("Beads agent profile: unknown (")
    first, _, rest = text.partition("\n\n")
    assert "\n" not in first and rest == hooks.rules()  # the rules, no command list, no pm show
    assert "## Commands" not in text and len(text) <= hooks.CAP


# ---------------------------------------------------------------- uncommitted records


def claude_transcript(path, *tool_inputs):
    """A Claude Code transcript whose assistant turns call tools with `tool_inputs`."""
    lines = [{"type": "user", "message": {"role": "user", "content": "go"}}]
    lines += [{"type": "assistant", "message": {"role": "assistant", "content": [
        {"type": "tool_use", "id": f"t{n}", "name": "Edit", "input": i}]}} for n, i in enumerate(tool_inputs)]
    path.write_text("\n".join(json.dumps(l) for l in lines) + "\n")
    return str(path)


def edit_sprint(repo):
    (repo.store / "sprints/demo-1.md").write_text(
        (repo.store / "sprints/demo-1.md").read_text().replace("Ship it.", "Ship it soon."))


def test_stop_blocks_on_a_record_this_session_edited(repo, tmp_path):
    edit_sprint(repo)
    t = claude_transcript(tmp_path / "t.jsonl", {"file_path": str(repo.records / "sprints/demo-1.md")})
    res = run(STOP, {"cwd": str(repo.root), "transcript_path": t, "stop_hook_active": False}, repo.env, repo.root)
    out = json.loads(res.stdout)
    assert out["decision"] == "block"
    assert "- records/sprints/demo-1.md" in out["reason"] and "bin/pm commit -m" in out["reason"]


def test_stop_blocks_on_a_new_file_named_in_a_codex_patch(repo, tmp_path):
    (repo.store / "docs").mkdir()
    (repo.store / "docs/2026-10-05-note.md").write_text("draft\n")
    t = tmp_path / "rollout.jsonl"
    t.write_text(json.dumps({"type": "response_item", "payload": {
        "type": "custom_tool_call", "name": "apply_patch",
        "input": "*** Begin Patch\n*** Add File: records/docs/2026-10-05-note.md\n+draft\n*** End Patch"}}) + "\n")
    res = run(STOP, {"cwd": str(repo.root), "transcript_path": str(t)}, repo.env, repo.root)
    assert "- records/docs/2026-10-05-note.md" in json.loads(res.stdout)["reason"]


def test_stop_passes_another_sessions_edit(repo, tmp_path):
    """A dirty record no tool call of this session names is another session's; a path only in a tool result (here
    the user's text) does not count either."""
    edit_sprint(repo)
    t = tmp_path / "t.jsonl"
    t.write_text(json.dumps({"type": "user", "message": {"content": "git status: sprints/demo-1.md"}}) + "\n")
    claude_transcript(tmp_path / "other.jsonl", {"file_path": str(repo.records / "sprints/demo-2.md")})
    t.write_text(t.read_text() + (tmp_path / "other.jsonl").read_text())
    res = run(STOP, {"cwd": str(repo.root), "transcript_path": str(t)}, repo.env, repo.root)
    assert res.returncode == 0 and res.stdout == ""


def test_stop_passes_a_clean_store(repo, tmp_path):
    t = claude_transcript(tmp_path / "t.jsonl", {"file_path": str(repo.records / "sprints/demo-1.md")})
    res = run(STOP, {"cwd": str(repo.root), "transcript_path": t}, repo.env, repo.root)
    assert res.returncode == 0 and res.stdout == ""


def test_stop_passes_when_stop_hook_active(repo, tmp_path):
    edit_sprint(repo)
    t = claude_transcript(tmp_path / "t.jsonl", {"command": "sed -i '' s/a/b/ records/sprints/demo-1.md"})
    res = run(STOP, {"cwd": str(repo.root), "transcript_path": t, "stop_hook_active": True}, repo.env, repo.root)
    assert res.returncode == 0 and res.stdout == ""


def test_stop_fails_open_without_a_transcript_or_git(repo, tmp_path, capsys):
    edit_sprint(repo)
    res = run(STOP, {"cwd": str(repo.root), "transcript_path": str(tmp_path / "gone.jsonl")}, repo.env, repo.root)
    assert res.returncode == 0 and res.stdout == "" and "no readable transcript" in res.stderr
    # The event's cwd is not a git checkout (pm itself runs in one with a config: outside one it fails hard).
    assert hooks.stop_reason({"cwd": str(tmp_path)}) is None and "git is unavailable" in capsys.readouterr().err
    res = subprocess.run(STOP, input="not json", cwd=repo.root, env=repo.env, capture_output=True, text=True)
    assert res.returncode == 0 and res.stdout == "" and "not JSON" in res.stderr


# ---------------------------------------------------------------- generated sections


def test_render_refuses_text_in_progress(repo):
    path = repo.store / "sprints/demo-1.md"
    path.write_text(path.read_text().replace("> Do not write here.\n", "> Do not write here.\n\nHalf done.\n"))
    res = repo.pm("render")
    assert res.returncode == 1
    assert "sprints/demo-1.md:" in res.stderr and "hand-written text in '## Progress'" in res.stderr, res.stderr
    line = int(res.stderr.split("sprints/demo-1.md:")[1].split(":")[0])
    assert path.read_text().splitlines()[line - 1] == "Half done."


def test_render_refuses_text_in_project_progress(repo):
    path = repo.store / "projects/demo.md"
    path.write_text(path.read_text().replace("> Where are we now, and what's next?\n",
                                             "> Where are we now, and what's next?\n\n### Next\n"))
    res = repo.pm("render")
    assert res.returncode == 1 and "hand-written text in '## Progress'" in res.stderr, res.stderr


def test_render_refuses_a_generated_heading_in_a_day(repo):
    path = repo.store / "days/2026-10-01.md"
    path.write_text(path.read_text() + "\n## Decisions await you\n\nNone.\n")
    res = repo.pm("render")
    assert res.returncode == 1 and "'## Decisions await you' is a section the page generates" in res.stderr


def test_render_accepts_prompt_lines_and_code_in_other_sections(repo):
    path = repo.store / "sprints/demo-1.md"
    path.write_text(path.read_text().replace("- It works.\n", "- It works.\n\n```\n## Progress\n## Docs\n```\n"))
    assert repo.pm("render").returncode == 0


def test_stop_reads_an_unstaged_rename_and_a_broken_transcript(repo, tmp_path):
    """An unstaged rename (` R new\\0old`) is one entry, and invalid UTF-8 or a non-object line does not stop the
    scan."""
    (repo.store / "sprints/demo-1.md").rename(repo.store / "sprints/demo-9.md")
    repo.git("add", "-N", "sprints/demo-9.md", cwd=repo.store)
    assert sorted(hooks.dirty(repo.store)) == ["sprints/demo-9.md"]
    t = tmp_path / "t.jsonl"
    claude_transcript(t, {"file_path": "records/sprints/demo-9.md"})
    t.write_bytes(b"\xff\xfe sprints/demo-9.md\n[\"sprints/demo-9.md\"]\n" + t.read_bytes())
    res = run(STOP, {"cwd": str(repo.root), "transcript_path": str(t)}, repo.env, repo.root)
    assert "- records/sprints/demo-9.md" in json.loads(res.stdout)["reason"], res.stderr



def test_session_start_warns_of_tasks_other_live_sessions_hold(repo):
    repo.set_issue("demo.1.2", status="in_progress", metadata={"claimed_by": "other", "claimed_at": "2026-10-01T12:00:00Z"})
    (Path(repo.env["CLAUDE_CONFIG_DIR"]) / "projects/-repo").mkdir(parents=True)
    (Path(repo.env["CLAUDE_CONFIG_DIR"]) / "projects/-repo/other.jsonl").write_text("{}\n")
    for sid, warned in (("me", True), ("other", False)):
        res = run(SESSION, {"hook_event_name": "SessionStart", "cwd": str(repo.root), "session_id": sid}, repo.env, repo.root)
        text = json.loads(res.stdout)["hookSpecificOutput"]["additionalContext"]
        assert ("warning: other live sessions hold these tasks" in text) is warned


def test_session_start_points_the_sessions_open_requests_at_its_current_inbox(repo):
    """A resumed session keeps its id but binds a new inbox socket; session start rewrites the stored inbox of its
    open requests (and the host) from the one Beads read pm show makes, and leaves other sessions' requests alone."""
    import socket
    repo.set_issue("demo.1.2", labels=["human"], metadata={"session": "me", "inbox": "/old/s", "inbox_host": "h"})
    repo.set_issue("demo.1.1", labels=["human"], metadata={"session": "other", "inbox": "/x/s", "inbox_host": "h"})
    env = dict(repo.env, CLAUDE_CODE_MESSAGING_SOCKET="/new/s")
    res = run(SESSION, {"hook_event_name": "SessionStart", "cwd": str(repo.root), "session_id": "me"}, env, repo.root)
    assert res.returncode == 0, res.stderr
    issues = repo.issues()
    assert issues["demo.1.2"]["metadata"] == {"session": "me", "inbox": "/new/s", "inbox_host": socket.gethostname()}
    assert issues["demo.1.1"]["metadata"]["inbox"] == "/x/s"
    assert [c for c in repo.bd_calls() if c[:1] == ["list"]] == [["list", "--all", "--json"]], "no extra Beads read"
    repo.log.write_text("")
    run(SESSION, {"hook_event_name": "SessionStart", "cwd": str(repo.root), "session_id": "me"}, env, repo.root)
    assert not [c for c in repo.bd_calls() if c[:1] == ["update"]], "an inbox already current is not rewritten"
