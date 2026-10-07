"""`pm prime` (the session and subagent context) and `pm hook stop` (blocks on this session's uncommitted records),
run as the runtimes run them: JSON on stdin, in a temp clone."""

from __future__ import annotations

import json
import re
import shutil
import subprocess
import sys

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


@pytest.mark.slow
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
    "warning: the scheduled push needs attention (pm where; it runs bin/pm push):",
    "  last run 2026-10-07 01:10 UTC failed: bd dolt push: remote rejected (non-fast-forward)",
    "  overdue: no run for 41 minutes; it runs every 10 minutes",
    "warning: other live sessions hold these tasks; do not start or delegate them:",
    *(f"  yeeef-agents-9va.6{n}.{n}  held by 7f3a9c0{n}, 2h ago, live" for n in range(8)),
    "today 2026-10-07: " + "Auto-summary pages, decision cards and feedback tracking shipped. " * 3,
    "site: https://pm.example.com (pm serve); a record's page is <site>/<its path under records/, without .md>.html",
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


@pytest.mark.slow
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


@pytest.mark.slow
def test_session_start_fails_open_with_one_line(repo):
    (repo.state).write_text("not json")  # the fake bd now fails, so pm show fails
    text = context_of(run(STATE, {"cwd": str(repo.root)}, repo.env, repo.root))
    _, located, shown = text.split("\n\n", 2)
    assert located.startswith("Locations from `bin/pm where` at session start:\n")  # pm where reads no Beads
    assert shown.startswith("pm show failed at session start (") and "\n" not in shown


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


def test_stop_passes_when_stop_hook_active(repo, tmp_path):
    edit_sprint(repo)
    t = claude_transcript(tmp_path / "t.jsonl", {"command": "sed -i '' s/a/b/ records/sprints/demo-1.md"})
    res = run(STOP, {"cwd": str(repo.root), "transcript_path": t, "stop_hook_active": True}, repo.env, repo.root)
    assert res.returncode == 0 and res.stdout == ""
