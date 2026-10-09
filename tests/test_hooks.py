"""`pm prime` (the session and subagent context) and `pm hook stop` (blocks on this session's uncommitted records),
run as the runtimes run them: JSON on stdin, in a temp clone."""

from __future__ import annotations

import json
import os
import re
import shutil
import socket
import subprocess
import sys
from pathlib import Path

import pytest

from conftest import GO_SUBAGENT, IMPL, PM, stop_services, write_config

from pm import hooks

IN_PROCESS = pytest.mark.impl("python", reason="checks Python pm's hooks module or parser in process")

STATE = [*PM, "prime", "--state", "--hook-json"]
SUBAGENT = [*PM, "prime", "--subagent", "--hook-json"]
STOP = [*PM, "hook", "stop"]


def rules_cmd(n):
    return [*PM, "prime", "--rules", str(n), "--hook-json"]


def run(cmd, event, env, cwd):
    return subprocess.run(cmd, input=json.dumps(event), env=env, cwd=cwd, capture_output=True, text=True, timeout=60)


def init_ready(repo):
    """Session start runs pm init, which needs `pm` on PATH to be the pm uv tool's, and installs a missing pm service
    (under the fake supervisor) on the config's port, since it drops $PORT: put the tool's bin dir first on PATH and
    install the service once, by hand, on a free port, never the user's."""
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        port = s.getsockname()[1]
    repo.env = dict(repo.env, PATH=f"{repo.env['UV_TOOL_BIN_DIR']}{os.pathsep}{repo.env['PATH']}", PORT=str(port))
    first = repo.pm("init")  # a clone set up once, as pm init leaves it
    assert first.returncode == 0, first.stderr


# ---------------------------------------------------------------- session context


def context_of(res, event="SessionStart"):
    assert res.returncode == 0, res.stderr
    out = json.loads(res.stdout)["hookSpecificOutput"]
    assert out["hookEventName"] == event
    return out["additionalContext"]


@pytest.mark.integration
def test_session_start_injects_rules_then_init_where_and_pm_show(repo):
    """One hook per rules chunk, then the state, each under Claude Code's per-hook cap, with pm show whole."""
    init_ready(repo)
    event = {"hook_event_name": "SessionStart", "cwd": str(repo.root)}
    got = [context_of(run(rules_cmd(n), event, repo.env, repo.root)) for n in range(1, len(hooks.STARTS) + 1)]
    assert got == hooks.chunks() and got[0].startswith("# pm rules (1 of 2): the introduction; ")
    text = context_of(run(STATE, event, repo.env, repo.root))
    shown = repo.pm("show").stdout.strip()
    ran, _, rest = text.partition("\n\n")
    # quiet when set up: nothing changed, the service is current
    assert ran.startswith(f"`pm init` at session start:\nalready set up: {repo.records} -> {repo.store}\n"), ran
    assert "installed" not in ran and "updated" not in ran and "wrote" not in ran, ran
    located, _, rest = rest.partition("\n\n")
    assert located == "Locations from `pm where` at session start:\n" + repo.pm("where").stdout.strip()
    assert f"checkout  {repo.root}  branch main, records link set up" in located
    header, _, body = rest.partition("\n\n")
    assert re.fullmatch(r"Project state from `pm show` at session start, \d{4}-\d\d-\d\d \d\d:\d\d UTC: .*"
                        r"run `pm show` again before stating project state to the owner\.", header)
    minute = lambda s: re.sub(r"\d\d:\d\d UTC", "hh:mm UTC", s)  # a stamp may cross a minute between two calls
    assert minute(body) == minute(shown) and "decision .1.2  Ask the owner  (sprint 1)" in body
    plain = repo.pm("prime")  # by hand: the rules whole and in order, then the state, no envelope
    assert plain.returncode == 0 and minute(plain.stdout.strip()) == minute(hooks.head() + "\n\n" + text)


TODAY_SHOW = "\n".join([  # pm show of a busy day: 2026-10-07 printed 7,085 characters; this one prints more
    "warning: the pm service's push needs attention (pm service status; pm service logs):",
    "  last run 2026-10-07 01:10 UTC failed: bd dolt push: remote rejected (non-fast-forward)",
    "  overdue: no run for 41 minutes; it runs every 10 minutes",
    "warning: other live sessions hold these tasks; do not start or delegate them:",
    *(f"  yeeef-agents-9va.6{n}.{n}  held by 7f3a9c0{n}, 2h ago, live" for n in range(8)),
    "today 2026-10-07: " + "Auto-summary pages, decision cards and feedback tracking shipped. " * 3,
    "site: https://pm.example.com (the pm service); a record's page is <site>/<its path under records/, without .md>.html",
    'feedback: when pm gets in your way, run pm feedback add --project <p> --text="…"',
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


@IN_PROCESS
def test_session_start_keeps_a_busy_days_pm_show_whole(tmp_path, monkeypatch):
    """The fixture is today-size: every warning and owner request line reaches the session, and nothing is cut."""
    assert len(TODAY_SHOW) > 7_085 and len(TODAY_WHERE) > 1_295
    monkeypatch.setattr(hooks, "SHOW", [sys.executable, "-c", f"print({TODAY_SHOW!r})"])
    monkeypatch.setattr(hooks, "INIT", [sys.executable, "-c", f"print({'already set up: ' + 'x' * 260!r})"])
    monkeypatch.setattr(hooks, "WHERE", [sys.executable, "-c", f"print({TODAY_WHERE!r})"])
    text = hooks.state(str(tmp_path))
    assert len(text) <= hooks.CAP and hooks.CUT not in text
    assert text.endswith(TODAY_SHOW)
    for line in TODAY_SHOW.splitlines():
        if line.startswith(("warning:", "  last run", "  overdue", "  yeeef-agents-9va")) or "await you" in line \
                or "-> bd show" in line:
            assert line in text.splitlines()
    assert all(len(c) <= hooks.CAP for c in hooks.chunks())  # each rules hook, too, reaches the session inline


@pytest.mark.integration
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
    init_ready(repo)  # after the worktree: main's sparse checkout, which pm init sets, would carry over to it
    text = context_of(run(STATE, {"hook_event_name": "SessionStart", "cwd": str(wt)}, repo.env, wt))
    assert (wt / "records").is_symlink() and (wt / "records").resolve() == repo.store.resolve(), text
    assert repo.git("status", "--porcelain", cwd=wt) == ""
    ran, located, rest = text.split("\n\n", 2)
    assert ran.startswith("`pm init` at session start:\n") and f"linked {wt / 'records'} -> {repo.store}" in ran
    assert f"checkout  {wt}  branch bridge, records link set up" in located
    assert rest.startswith("Project state from `pm show` at session start, ")
    assert "decision .1.2  Ask the owner  (sprint 1)" in rest


@IN_PROCESS
def test_session_start_runs_init_without_port(tmp_path, monkeypatch):
    """$PORT in a session's environment is no request to move the clone's service: session start's init never sees it."""
    monkeypatch.setenv("PORT", "8123")
    said = hooks.init(str(tmp_path), [sys.executable, "-c", "import os; print(os.environ.get('PORT', 'unset'))"])
    assert said == "`pm init` at session start:\nunset\n\n", said


@pytest.mark.integration
@pytest.mark.impl("python", reason="Go pm's commands reach the work store only through the service, so its session "
                  "start starts a down one (test_session_start_starts_a_down_service_once)")
def test_session_start_reports_a_down_service_and_a_typed_init_restarts_it(repo, tmp_path):
    """Session start installs only a missing service: a down one is reported, never restarted within its budget;
    pm init typed by a person restarts it, on the port its unit serves on although the port's last connections
    linger in TIME_WAIT."""
    init_ready(repo)
    port = int(repo.env["PORT"])
    repo.env = {k: v for k, v in repo.env.items() if k != "PORT"}
    stop_services(tmp_path)  # the process dies; the fake supervisor still holds the unit
    calls = (tmp_path / "sched.log").read_text()
    text = context_of(run(STATE, {"hook_event_name": "SessionStart", "cwd": str(repo.root)}, repo.env, repo.root))
    ran = text.partition("\n\n")[0]
    assert f"left the installed pm service as it is (session start never restarts it): down: nothing answers on :{port}" \
        in ran, ran
    assert (tmp_path / "sched.log").read_text().count("bootstrap") == calls.count("bootstrap"), "no restart"
    res = repo.pm("init")
    assert res.returncode == 0 and f"updated the pm service: " in res.stdout, (res.stdout, res.stderr)
    assert f"serving http://localhost:{port} " in res.stdout


@pytest.mark.integration
@pytest.mark.impl("go", reason="Python pm's session start leaves a down service to a typed pm init")
def test_session_start_starts_a_down_service_once(repo, tmp_path):
    """Every Go pm store command needs the service, so session start starts an installed one that does not answer,
    under the clone's install lock: once, however many sessions start at once. A running one is left as it is."""
    init_ready(repo)
    port = int(repo.env["PORT"])
    repo.env = {k: v for k, v in repo.env.items() if k != "PORT"}
    stop_services(tmp_path)  # the process dies; the fake supervisor still holds the unit
    calls = (tmp_path / "sched.log").read_text()
    starts = [subprocess.Popen([*PM, "init", "--session-start"], cwd=repo.root, env=repo.env, text=True,
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE) for _ in range(3)]
    outs = [p.communicate(timeout=60) for p in starts]
    assert all(p.returncode == 0 for p in starts), outs
    assert sum("restarted the pm service" in out for out, _ in outs) == 1, outs
    kicks = (tmp_path / "sched.log").read_text()[len(calls):]
    assert kicks.count("kickstart") + kicks.count('"restart"') == 1, kicks
    assert repo.pm("show", "repo-demo.1").returncode == 0, "the store answers again"
    text = context_of(run(STATE, {"hook_event_name": "SessionStart", "cwd": str(repo.root)}, repo.env, repo.root))
    assert "restarted the pm service" not in text.partition("\n\n")[0], "a running service is left as it is"
    assert f":{port}" in text


@pytest.mark.integration
def test_session_start_fails_open_with_one_line(repo):
    init_ready(repo)
    if IMPL == "python":
        (repo.state).write_text("not json")  # the fake bd now fails, so pm show fails
    else:  # Go pm show reads its work store, which is gone
        shutil.rmtree(repo.root / ".pm/store/work")
    text = context_of(run(STATE, {"cwd": str(repo.root)}, repo.env, repo.root))
    _, located, shown = text.split("\n\n", 2)
    assert located.startswith("Locations from `pm where` at session start:\n")  # pm where reads no Beads
    assert shown.startswith("pm show failed at session start (") and "\n" not in shown


def subcommands():
    from pm.cli import parser
    return next(a for a in parser()._subparsers._group_actions if a.dest == "cmd").choices


def listed_nouns():
    line = hooks.commands().split("\n")[-1]
    assert line.startswith("`pm` nouns: ") and line.endswith(".")
    return re.findall(r"`(\w+)`", line[len("`pm` nouns: "):])


@IN_PROCESS
def test_prime_lists_every_agent_command_from_the_parser():
    listed = listed_nouns()
    assert listed == [c for c in subcommands() if c not in {"prime", "hook", "push"}]
    assert "prime" not in listed and "show" in listed
    assert hooks.commands().startswith("# Commands\n\n")
    assert hooks.commands().count("\n") == 2  # compact: a heading and the nouns; prime.md points at --help
    assert "for more, run `pm <noun> [cmd] --help`." in hooks.rules()  # the pointer the compact list relies on


@IN_PROCESS
def test_pm_init_is_the_one_install_command_and_session_start_runs_it():
    """No `pm setup`: pm --help lists init only, and the session-start state hook runs pm init (the worktree test
    above shows it setting up a new worktree)."""
    from pm.cli import parser
    assert "setup" not in subcommands() and "init" in subcommands()
    listing = parser().format_help()
    assert not re.search(r"^ {4}setup\b|[{,]setup[,}]|pm setup", listing, re.M), listing
    assert hooks.INIT[-2:] == ["init", "--session-start"]
    res = subprocess.run([*PM, "setup"], capture_output=True, text=True)
    assert res.returncode == 2 and "invalid choice: 'setup'" in res.stderr, res.stderr


@IN_PROCESS
def test_rules_chunks_fit_the_cap_and_add_up_to_the_rules():
    """Each chunk reaches the session inline (a hook over CAP arrives as a 2 KB preview), and the chunks without
    their titles are the rules and the command list: nothing lost, nothing twice. A failure here means prime.md
    outgrew its chunks: move a heading in hooks.STARTS, or add one and its hook entries."""
    cs = hooks.chunks()
    assert len(cs) == len(hooks.STARTS) == 2
    assert all(len(c) <= hooks.CAP for c in cs), [len(c) for c in cs]
    titles, bodies = zip(*(c.split("\n\n", 1) for c in cs))
    assert "\n\n".join(bodies) == hooks.head()
    assert [b.split("\n", 1)[0] for b in bodies] == list(hooks.STARTS)
    for n, t in enumerate(titles, 1):  # hooks arrive in any order, so each title names its place and its sections
        assert t.startswith(f"# pm rules ({n} of 2): ") and "\n" not in t
    assert "What — 1. The layers" in titles[0] and "How; Commands" in titles[1]


def test_subagent_start_envelope(tmp_path):
    """The rules chunks and the git line (Python: the Beads profile line), each a SubagentStart envelope; no pm
    show."""
    subprocess.run(["git", "init", "-q", str(tmp_path)], check=True)
    write_config(tmp_path)
    (tmp_path / "bin").mkdir()
    (tmp_path / "bin/git").symlink_to(shutil.which("git"))
    event = {"hook_event_name": "SubagentStart", "cwd": str(tmp_path)}
    env = {"PATH": str(tmp_path / "bin")}  # git for pm's config check; no bd
    text = context_of(run(SUBAGENT, event, env, tmp_path), "SubagentStart")
    if IMPL == "go":
        assert text == GO_SUBAGENT
    else:
        assert text.startswith("Beads agent profile: unknown (") and "\n" not in text
    got = [context_of(run(rules_cmd(n), event, env, tmp_path), "SubagentStart") for n in range(1, len(hooks.STARTS) + 1)]
    assert got == hooks.chunks() and all(len(c) <= hooks.CAP for c in got)


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
    assert "- records/sprints/demo-1.md" in out["reason"] and "pm commit -m" in out["reason"]


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
