"""`pm prime` (the session and subagent context), `pm hook stop` (blocks on this session's uncommitted records), and
the render check on generated sections. The hooks run as the runtimes run them: JSON on stdin, in a temp clone."""

from __future__ import annotations

import json
import re
import shutil
import subprocess
import sys
from pathlib import Path

import pytest

from conftest import PM, write_config

from pm import hooks

RULES = [*PM, "prime", "--rules", "--hook-json"]
STATE = [*PM, "prime", "--state", "--hook-json"]
SUBAGENT = [*PM, "prime", "--subagent", "--hook-json"]
STOP = [*PM, "hook", "stop"]


def run(cmd, event, env, cwd):
    return subprocess.run(cmd, input=json.dumps(event), env=env, cwd=cwd, capture_output=True, text=True, timeout=60)


# ---------------------------------------------------------------- session context


def context_of(res, event="SessionStart"):
    assert res.returncode == 0, res.stderr
    out = json.loads(res.stdout)["hookSpecificOutput"]
    assert out["hookEventName"] == event
    return out["additionalContext"]


def test_session_start_injects_rules_then_setup_where_and_pm_show(repo):
    """Two hooks, each under Claude Code's per-hook cap: the rules never cut, then the state with pm show whole."""
    assert repo.pm("setup").returncode == 0  # a clone set up once, as bin/pm setup leaves it
    event = {"hook_event_name": "SessionStart", "cwd": str(repo.root)}
    rules = context_of(run(RULES, event, repo.env, repo.root))
    assert rules == hooks.rules() + "\n\n" + hooks.commands() and rules.startswith("# pm rules\n")
    text = context_of(run(STATE, event, repo.env, repo.root))
    shown = repo.pm("show").stdout.strip()
    ran, _, rest = text.partition("\n\n")
    assert ran.startswith(f"`bin/pm setup` at session start:\nalready set up: {repo.records} -> {repo.store}\n")
    located, _, rest = rest.partition("\n\n")
    assert located == "Locations from `bin/pm where` at session start:\n" + repo.pm("where").stdout.strip()
    assert f"checkout  {repo.root}  branch main, records link set up" in located
    header, _, body = rest.partition("\n\n")
    assert re.fullmatch(r"Project state from `bin/pm show` at session start, \d{4}-\d\d-\d\d \d\d:\d\d UTC: .*"
                        r"run `bin/pm show` again before stating project state to the owner\.", header)
    assert body == shown and "Sprint 1: First" in body
    plain = repo.pm("prime")  # by hand: both, no envelope
    assert plain.returncode == 0 and plain.stdout.strip() == rules + "\n\n" + text


TODAY_SHOW = "\n".join([  # pm show of a busy day: 2026-10-07 printed 7,085 characters; this one prints more
    "warning: the pm service's push needs attention (pm service status; pm service logs):",
    "  last run 2026-10-07 01:10 UTC failed: bd dolt push: remote rejected (non-fast-forward)",
    "  overdue: no run for 41 minutes; it runs every 10 minutes",
    "warning: other live sessions hold these tasks; do not start or delegate them:",
    *(f"  yeeef-agents-9va.6{n}.{n}  held by 7f3a9c0{n}, 2h ago, live" for n in range(8)),
    "today 2026-10-07: " + "Auto-summary pages, decision cards and feedback tracking shipped. " * 3,
    "site: https://pm.example.com (the pm service); a record's page is <site>/<its path under records/, without .md>.html",
    'feedback: when pm gets in your way, run pm feedback add --project <p> --text "…"',
    *(line for p in range(5) for line in (
        f"project-{p}  yeeef-agents-p{p}  " + "One shared record for agents and the owner, kept current. " * 2,
        "decisions await you (2):",
        f"  .{p}1  Choose the store layout for project {p}  (sprint {p}) -> bd show yeeef-agents-p{p}.1",
        f"  .{p}2  Pick the push schedule for project {p}  (sprint {p}) -> bd show yeeef-agents-p{p}.2",
        "actions await you (1):",
        f"  .{p}3  Review PR #{50 + p}  (sprint {p}) -> bd show yeeef-agents-p{p}.3")),
    *(line for p in range(5) for line in (
        f"project-{p}  yeeef-agents-p{p}  sprints and decisions:",
        *(f"  .{p}{s} Sprint {s}: " + "Ship the part of the harness this sprint owns. " * 2 for s in range(3)),
        *(f"    .{p}{s}.{t}  open  Task {t} of the sprint, with a title of a usual length" for s in range(1) for t in range(4)),
        "decisions (last 1):",
        *(f"  2026-10-0{d} agent sprint {p}  " + "A decision body cut to its first hundred characters, as pm show prints it…"
          for d in range(1)))),
])
TODAY_WHERE = "\n".join(f"{k:<9} /home/someone/workspace/yeeef-agents/{k}  " + "state of this location, " * 2
                        for k in ("store", "checkout", "beads", "hooks", "codex", "claude", "push", "site", "remote",
                                  "records", "worktree", "summary", "inbox"))


def test_session_start_keeps_a_busy_days_pm_show_whole(tmp_path, monkeypatch):
    """The fixture is today-size: every warning and owner request line reaches the session, and nothing is cut."""
    assert len(TODAY_SHOW) > 7_085 and len(TODAY_WHERE) > 1_295
    monkeypatch.setattr(hooks, "SHOW", [sys.executable, "-c", f"print({TODAY_SHOW!r})"])
    monkeypatch.setattr(hooks, "SETUP", [sys.executable, "-c", f"print({'already set up: ' + 'x' * 260!r})"])
    monkeypatch.setattr(hooks, "WHERE", [sys.executable, "-c", f"print({TODAY_WHERE!r})"])
    text = hooks.state(str(tmp_path))
    assert len(text) <= hooks.CAP and hooks.CUT not in text
    assert text.endswith(TODAY_SHOW)
    for line in TODAY_SHOW.splitlines():
        if line.startswith(("warning:", "  last run", "  overdue", "  yeeef-agents-9va")) or "await you" in line \
                or "-> bd show" in line:
            assert line in text.splitlines()
    rules = hooks.head()
    assert len(rules) <= hooks.CAP and hooks.CUT not in rules  # the rules hook is never cut


@pytest.mark.parametrize("tracked", [False, True], ids=["no-records", "mains-tracked-copy"])
def test_session_start_sets_up_a_worktree_post_checkout_skipped(repo, tracked):
    """Claude Code's worktrees: added with --no-checkout, then reset, so git never runs post-checkout. With main
    tracking a copy of records/ (and ignoring /records for the link), the reset leaves that copy where the link goes."""
    if tracked:
        repo.records.unlink()
        (repo.records / "sprints").mkdir(parents=True)
        (repo.records / "sprints/demo-1.md").write_text("copy\n")
        repo.git("add", "-f", "records")
        repo.git("commit", "-qm", "copy records")
        shutil.rmtree(repo.records)
        repo.records.symlink_to(repo.store)
    wt = repo.root.parent / "bridge"
    repo.git("worktree", "add", "-q", "--no-checkout", "-b", "bridge", str(wt))
    repo.git("reset", "-q", "--hard", cwd=wt)
    assert (wt / "records").is_dir() == tracked and not (wt / "records").is_symlink()
    text = context_of(run(STATE, {"hook_event_name": "SessionStart", "cwd": str(wt)}, repo.env, wt))
    assert (wt / "records").is_symlink() and (wt / "records").resolve() == repo.store.resolve()
    assert repo.git("status", "--porcelain", cwd=wt) == ""
    ran, located, rest = text.split("\n\n", 2)
    assert ran.startswith("`bin/pm setup` at session start:\n") and f"linked {wt / 'records'} -> {repo.store}" in ran
    assert f"checkout  {wt}  branch bridge, records link set up" in located
    assert rest.startswith("Project state from `bin/pm show` at session start, ") and "Sprint 1: First" in rest


def test_session_start_setup_fails_open_with_one_line(tmp_path):
    note = hooks.setup(str(tmp_path), [sys.executable, "-c", "import sys; sys.exit('no records branch')"])
    assert note == "pm setup failed at session start (no records branch); run `bin/pm setup` by hand.\n\n"


def test_session_start_where_fails_open_with_one_line(tmp_path):
    note = hooks.where(str(tmp_path), [sys.executable, "-c", "import sys; sys.exit('no records store')"])
    assert note == "pm where failed at session start (no records store); run `bin/pm where` by hand.\n\n"


def test_session_start_cuts_long_output_at_a_line(tmp_path):
    long = "\n".join(f"line {n} " + "x" * 90 for n in range(200))
    text = hooks.context(str(tmp_path), [sys.executable, "-c", f"print({long!r})"])
    assert len(text) <= hooks.CAP
    assert text.endswith(hooks.CUT)
    assert text[:-len(hooks.CUT)].endswith("x" * 90)  # whole lines only


def test_session_start_fails_open_with_one_line(repo):
    (repo.state).write_text("not json")  # the fake bd now fails, so pm show fails
    text = context_of(run(STATE, {"cwd": str(repo.root)}, repo.env, repo.root))
    _, located, shown = text.split("\n\n", 2)
    assert located.startswith("Locations from `bin/pm where` at session start:\n")  # pm where reads no Beads
    assert shown.startswith("pm show failed at session start (") and "\n" not in shown


def test_session_state_stays_within_the_cap_cutting_pm_show_last(tmp_path, monkeypatch):
    long = "\n".join(f"line {n} " + "x" * 90 for n in range(200))
    monkeypatch.setattr(hooks, "SHOW", [sys.executable, "-c", f"print({long!r})"])
    monkeypatch.setattr(hooks, "SETUP", [sys.executable, "-c", "print('already set up')"])
    monkeypatch.setattr(hooks, "WHERE", [sys.executable, "-c", "print('store  .records')"])
    text = hooks.state(str(tmp_path))
    assert len(text) <= hooks.CAP and text.endswith(hooks.CUT)
    ran, located, rest = text.split("\n\n", 2)
    assert ran.endswith("already set up") and located.endswith("store  .records")
    assert rest.startswith("Project state") and "\nline 0 " in rest  # the cut takes the end of pm show


def subcommands():
    from pm.cli import parser
    return next(a for a in parser()._subparsers._group_actions if a.dest == "cmd").choices


def listed_nouns():
    line = hooks.commands().split("\n")[-1]
    assert line.startswith("`pm` nouns: ") and line.endswith(".")
    return re.findall(r"`(\w+)`", line[len("`pm` nouns: "):])


def test_prime_lists_every_agent_command_from_the_parser():
    listed = listed_nouns()
    assert listed == [c for c in subcommands() if c not in {"prime", "hook", "push"}]
    assert "prime" not in listed and "show" in listed
    assert hooks.commands().startswith("## Commands\n\n")
    assert hooks.commands().count("\n") == 2  # compact: a heading and the nouns; prime.md points at --help
    assert "Run `pm <noun> --help` for its commands and flags." in hooks.rules()


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


def sentences(text):
    """Each sentence of prime.md's bullets, paragraphs and table cells, with a code span counted as one word and an
    arrow (a step in an ordered line) as none."""
    for line in text.splitlines():
        if line.startswith("#") or re.fullmatch(r"\|[-|]+\|", line):
            continue
        for cell in line.strip("| ").split(" | ") if line.startswith("|") else [line.lstrip("- ")]:
            cell = re.sub(r"`[^`]*`", "CODE", cell).replace("→", " ")
            yield from (s for s in re.split(r"(?<=[.?!])\s+", cell) if s.strip())


def test_prime_md_sentences_are_at_most_20_words():
    """ASD-STE100: at most 20 words in an instruction sentence; pm's rules hold only instructions and short facts."""
    long = [(len(s.split()), s) for s in sentences(hooks.rules()) if len(s.split()) > 20]
    assert not long, long


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
    assert "\n" not in first and rest == hooks.head()  # no pm show
    assert len(text) <= hooks.CAP


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
    res = repo.pm("check")
    assert res.returncode == 1
    assert "sprints/demo-1.md:" in res.stderr and "hand-written text in '## Progress'" in res.stderr, res.stderr
    line = int(res.stderr.split("sprints/demo-1.md:")[1].split(":")[0])
    assert path.read_text().splitlines()[line - 1] == "Half done."


def test_render_refuses_text_in_project_progress(repo):
    path = repo.store / "projects/demo.md"
    path.write_text(path.read_text().replace("> Where are we now, and what's next?\n",
                                             "> Where are we now, and what's next?\n\n### Next\n"))
    res = repo.pm("check")
    assert res.returncode == 1 and "hand-written text in '## Progress'" in res.stderr, res.stderr


def test_render_refuses_a_generated_heading_in_a_day(repo):
    path = repo.store / "days/2026-10-01.md"
    path.write_text(path.read_text() + "\n## Decisions await you\n\nNone.\n")
    res = repo.pm("check")
    assert res.returncode == 1 and "'## Decisions await you' is a section the page generates" in res.stderr


def test_render_accepts_prompt_lines_and_code_in_other_sections(repo):
    path = repo.store / "sprints/demo-1.md"
    path.write_text(path.read_text().replace("- It works.\n", "- It works.\n\n```\n## Progress\n## Docs\n```\n"))
    assert repo.pm("check").returncode == 0


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
        res = run(STATE, {"hook_event_name": "SessionStart", "cwd": str(repo.root), "session_id": sid}, repo.env, repo.root)
        text = json.loads(res.stdout)["hookSpecificOutput"]["additionalContext"]
        assert ("warning: other live sessions hold these tasks" in text) is warned


def test_session_start_points_the_sessions_open_requests_at_its_current_inbox(repo):
    """A resumed session keeps its id but binds a new inbox socket; session start rewrites the stored inbox of its
    open requests (and the host) from the one Beads read pm show makes, and leaves other sessions' requests alone."""
    import socket
    repo.set_issue("demo.1.2", labels=["human"], metadata={"session": "me", "inbox": "/old/s", "inbox_host": "h"})
    repo.set_issue("demo.1.1", labels=["human"], metadata={"session": "other", "inbox": "/x/s", "inbox_host": "h"})
    env = dict(repo.env, CLAUDE_CODE_MESSAGING_SOCKET="/new/s")
    res = run(STATE, {"hook_event_name": "SessionStart", "cwd": str(repo.root), "session_id": "me"}, env, repo.root)
    assert res.returncode == 0, res.stderr
    issues = repo.issues()
    assert issues["demo.1.2"]["metadata"] == {"session": "me", "inbox": "/new/s", "inbox_host": socket.gethostname()}
    assert issues["demo.1.1"]["metadata"]["inbox"] == "/x/s"
    assert [c for c in repo.bd_calls() if c[:1] == ["list"]] == [["list", "--all", "--json"]], "no extra Beads read"
    repo.log.write_text("")
    run(STATE, {"hook_event_name": "SessionStart", "cwd": str(repo.root), "session_id": "me"}, env, repo.root)
    assert not [c for c in repo.bd_calls() if c[:1] == ["update"]], "an inbox already current is not rewritten"
