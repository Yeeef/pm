"""`pm prime` (the session and subagent context) and `pm hook stop` (blocks on this session's uncommitted records),
run as the runtimes run them: JSON on stdin, in a temp clone."""

from __future__ import annotations

import json
import re
import shutil
import socket
import subprocess

import pytest

from conftest import CAP, MACHINERY, PM, RULE_STARTS, SUBAGENT_RULE, chunks, head, nouns, rules, stop_services, \
    write_config

STATE = [*PM, "prime", "--state", "--hook-json"]
SUBAGENT = [*PM, "prime", "--subagent", "--hook-json"]
STOP = [*PM, "hook", "stop"]


def rules_cmd(n):
    return [*PM, "prime", "--rules", str(n), "--hook-json"]


def run(cmd, event, env, cwd):
    return subprocess.run(cmd, input=json.dumps(event), env=env, cwd=cwd, capture_output=True, text=True, timeout=60)


def init_ready(repo):
    """Session start runs pm init, which installs a missing pm service (under the fake supervisor) on the config's
    port, since it drops $PORT: install the service once, by hand, on a free port, never the user's."""
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        port = s.getsockname()[1]
    repo.env = dict(repo.env, PORT=str(port))
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
    got = [context_of(run(rules_cmd(n), event, repo.env, repo.root)) for n in range(1, len(RULE_STARTS) + 1)]
    assert got == chunks() and got[0].startswith("# pm rules (1 of 2): the introduction; ")
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
    assert plain.returncode == 0 and minute(plain.stdout.strip()) == minute(head() + "\n\n" + text)


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


@pytest.mark.integration
def test_session_start_starts_a_down_service_once(repo, tmp_path):
    """Every pm store command needs the service, so session start starts an installed one that does not answer,
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
    shutil.rmtree(repo.root / ".pm/store/work")  # pm show reads the work store, which is gone
    text = context_of(run(STATE, {"cwd": str(repo.root)}, repo.env, repo.root))
    _, located, shown = text.split("\n\n", 2)
    assert located.startswith("Locations from `pm where` at session start:\n")  # pm where reads no work store
    assert shown.startswith("pm show failed at session start (") and "\n" not in shown


def test_prime_lists_every_agent_command_pm_help_lists(repo):
    """pm prime's last line names every noun pm --help lists but the runtimes' and the scheduler's (MACHINERY), in its
    order; prime.md points at --help for the rest."""
    last = head().split("\n")[-1]
    shown = repo.pm("prime", "--rules", str(len(RULE_STARTS)))
    assert shown.stdout.rstrip("\n").split("\n")[-1] == last, shown.stderr
    listed = re.findall(r"`(\w+)`", last[len("`pm` nouns: "):])
    assert listed == [n for n in nouns() if n not in MACHINERY] and "show" in listed and "prime" not in listed
    assert "for more, run `pm <noun> [cmd] --help`." in rules()  # the pointer the compact list relies on


def test_pm_init_is_the_one_install_command():
    """No `pm setup`: pm --help lists init only."""
    assert "setup" not in nouns() and "init" in nouns()
    res = subprocess.run([*PM, "setup"], cwd="/", capture_output=True, text=True)
    assert res.returncode == 2 and "invalid choice: 'setup'" in res.stderr, res.stderr


def test_rules_chunks_fit_the_cap_and_add_up_to_the_rules():
    """Each chunk reaches the session inline (a hook over CAP arrives as a 2 KB preview), and the chunks without
    their titles are the rules and the command list: nothing lost, nothing twice. A failure here means prime.md
    outgrew its chunks: move a heading in RULE_STARTS and pm's, or add one and its hook entries."""
    cs = chunks()
    assert len(cs) == len(RULE_STARTS) == 2
    assert all(len(c) <= CAP for c in cs), [len(c) for c in cs]
    titles, bodies = zip(*(c.split("\n\n", 1) for c in cs))
    assert "\n\n".join(bodies) == head()
    assert [b.split("\n", 1)[0] for b in bodies] == list(RULE_STARTS)
    for n, t in enumerate(titles, 1):  # hooks arrive in any order, so each title names its place and its sections
        assert t.startswith(f"# pm rules ({n} of 2): ") and "\n" not in t
    assert "What — 1. The layers" in titles[0] and "How; Commands" in titles[1]


def test_subagent_start_envelope(tmp_path):
    """The rules chunks and the git line, each a SubagentStart envelope; no pm show."""
    subprocess.run(["git", "init", "-q", str(tmp_path)], check=True)
    write_config(tmp_path)
    (tmp_path / "bin").mkdir()
    (tmp_path / "bin/git").symlink_to(shutil.which("git"))
    event = {"hook_event_name": "SubagentStart", "cwd": str(tmp_path)}
    env = {"PATH": str(tmp_path / "bin")}  # git for pm's config check
    text = context_of(run(SUBAGENT, event, env, tmp_path), "SubagentStart")
    assert text == SUBAGENT_RULE
    got = [context_of(run(rules_cmd(n), event, env, tmp_path), "SubagentStart") for n in range(1, len(RULE_STARTS) + 1)]
    assert got == chunks() and all(len(c) <= CAP for c in got)


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
