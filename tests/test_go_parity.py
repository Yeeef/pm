"""Go pm against Python pm: `pm prime` in every mode byte-identical, `pm hook stop` and the config check identical,
and every command's `--help` identical after whitespace normalisation. Both run the same argv in the same temp clone
with the same environment; each test compares stdout, stderr and the exit code.

PM_GO names the Go binary, built with buildinfo.Version set to this pm's version (the parity CI job builds it); without
PM_GO this module is skipped. The reference help layout is Python 3.13's argparse (3.12 prints `-n LINES, --lines
LINES`), so the parity job runs Python 3.13. COLUMNS is set wide so that argparse wraps no line, as Go pm never does.

What the outputs cannot share yet: `pm prime --state` and plain `pm prime` run `pm init`, `pm where` and `pm show`,
which Go pm does not have yet; they are compared where those three fail at the config check, in a directory outside
any git repo."""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
from pathlib import Path

import pytest

from conftest import PM, integration_only, write_config
from work_items import items as work_items

from pm import __version__, hooks
from pm.cli import parser

GO = os.environ.get("PM_GO")
pytestmark = pytest.mark.skipif(not GO, reason="PM_GO names no Go pm binary; the parity CI job builds one")


def run(repo, impl, *args, event=None, cwd=None, env=None):
    """pm (python or go) with args in the repo; the event, as JSON, on stdin when given."""
    cmd = [*PM] if impl == "python" else [GO]
    feed = {"stdin": subprocess.DEVNULL} if event is None else {"input": event if isinstance(event, str) else json.dumps(event)}
    res = subprocess.run([*cmd, *args], cwd=cwd or repo.root, env=dict(env or repo.env, COLUMNS="10000"),
                         capture_output=True, text=True, timeout=60, **feed)
    return res.returncode, res.stdout, res.stderr


def same(repo, *args, **kw):
    """Run both and assert the same exit code, stdout and stderr; return Python's."""
    py, go = run(repo, "python", *args, **kw), run(repo, "go", *args, **kw)
    assert go == py
    return py


# ---------------------------------------------------------------- pm prime


@pytest.mark.parametrize("n", range(1, len(hooks.STARTS) + 1))
@pytest.mark.parametrize("hook_json", [False, True])
def test_prime_rules_chunk(repo, n, hook_json):
    args = ["prime", "--rules", str(n), *(["--hook-json"] if hook_json else [])]
    code, out, _ = same(repo, *args, event={"hook_event_name": "SubagentStart"} if hook_json else None)
    assert code == 0 and out.strip()


def test_prime_rules_hook_json_names_session_start_without_an_event_name(repo):
    code, out, _ = same(repo, "prime", "--rules", "1", "--hook-json", event="")
    assert json.loads(out)["hookSpecificOutput"]["hookEventName"] == "SessionStart"


def bin_with(tmp_path, name, script):
    """A PATH dir holding one program `name` running the sh script."""
    d = tmp_path / f"bin-{name}-{abs(hash(script))}"
    d.mkdir()
    (d / name).write_text("#!/bin/sh\n" + script + "\n")
    (d / name).chmod(0o755)
    return d


@pytest.mark.parametrize("case", ["conservative", "team-maintainer", "unset", "failing", "silent", "not-json",
                                  "extra-data", "missing", "no-cwd"])
@pytest.mark.parametrize("hook_json", [False, True])
def test_prime_subagent(repo, tmp_path, case, hook_json):
    env, event = dict(repo.env), {"cwd": str(repo.root), "hook_event_name": "SubagentStart"}
    if case == "team-maintainer":
        (repo.root / ".git/fake-bd-agent.profile").write_text("team-maintainer")
    elif case == "unset":
        (repo.root / ".git/fake-bd-agent.profile").write_text("")
    elif case == "failing":
        env["PATH"] = f"{bin_with(tmp_path, 'bd', 'echo no database here >&2; exit 3')}{os.pathsep}{env['PATH']}"
    elif case == "silent":
        env["PATH"] = f"{bin_with(tmp_path, 'bd', 'exit 0')}{os.pathsep}{env['PATH']}"
    elif case == "not-json":
        env["PATH"] = f"{bin_with(tmp_path, 'bd', 'echo; echo No database found')}{os.pathsep}{env['PATH']}"
    elif case == "extra-data":
        env["PATH"] = f"{bin_with(tmp_path, 'bd', 'echo {\\"value\\": 1} more')}{os.pathsep}{env['PATH']}"
    elif case == "missing":  # a PATH with git alone
        only_git = tmp_path / "bin-git"
        only_git.mkdir()
        (only_git / "git").symlink_to(shutil.which("git"))
        env["PATH"] = str(only_git)
    elif case == "no-cwd":
        event["cwd"] = str(tmp_path / "gone")
        if not hook_json:
            pytest.skip("only the hook input names a cwd")
    code, out, _ = same(repo, "prime", "--subagent", *(["--hook-json"] if hook_json else []),
                        event=event if hook_json else None, env=env)
    assert code == 0 and "Beads agent profile" in out


@pytest.mark.integration  # Python pm's session-start path starts `python -m pm.cli init`, which fails at once here
@pytest.mark.parametrize("mode", [["--state"], []])
def test_prime_state_where_init_where_and_show_fail_at_the_config_check(repo, tmp_path, mode):
    outside = tmp_path / "outside"
    outside.mkdir()
    code, out, _ = same(repo, "prime", *mode, "--hook-json", event={"cwd": str(outside), "session_id": "s1"})
    text = json.loads(out)["hookSpecificOutput"]["additionalContext"]
    assert code == 0 and "pm init failed at session start (error: " in text and "is not in a git worktree" in text


# ---------------------------------------------------------------- config check


def test_config_check_without_a_config(repo, tmp_path):
    bare = tmp_path / "plain"
    bare.mkdir()
    subprocess.run(["git", "init", "-q", str(bare)], check=True)
    code, _, err = same(repo, "prime", "--rules", "1", cwd=bare)
    assert code == 1 and "has no .pm/config.toml" in err


def test_config_check_outside_git(repo, tmp_path):
    code, _, err = same(repo, "hook", "stop", event={}, cwd=tmp_path)
    assert code == 1 and "is not in a git worktree" in err


def test_config_check_names_unknown_missing_and_mistyped_keys(repo):
    (repo.root / ".pm/config.toml").write_text('version = "x"\nport = "8000"\nextra = 1\nzeta = true\nremote = 5\n')
    code, _, err = same(repo, "prime", "--rules", "1", env=dict(repo.env, PM_LAUNCHED="x"))  # Python's launcher: no launch
    assert code == 1 and "unknown keys extra, zeta; missing keys main_branch; wrong types for port" in err


def test_config_check_names_a_key_made_a_table(repo):
    (repo.root / ".pm/config.toml").write_text('version.a = 1\nremote = "origin"\nmain_branch = "main"\nport = 8000\n')
    code, _, err = same(repo, "prime", "--rules", "1", env=dict(repo.env, PM_LAUNCHED="x"))
    assert code == 1 and "wrong types for version (want str)" in err


def test_config_check_refuses_another_pin(repo):
    write_config(repo.root, version="0.0.9")
    code, _, err = same(repo, "hook", "stop", event={}, env=dict(repo.env, PM_LAUNCHED="0.0.9"))
    assert code == 1 and f"this repo pins pm 0.0.9" in err and f"pm {__version__} is running" in err


# ---------------------------------------------------------------- pm hook stop


def transcript(path, *lines):
    path.write_text("".join(json.dumps(l, ensure_ascii=False) + "\n" for l in lines))
    return str(path)


def tool_use(inp):
    return {"type": "assistant", "message": {"role": "assistant", "content": [
        {"type": "tool_use", "id": "t1", "name": "Edit", "input": inp}]}}


def edit_sprint(repo):
    path = repo.store / "sprints/demo-1.md"
    path.write_text(path.read_text().replace("Ship it.", "Ship it soon."))


def test_stop_blocks_on_a_record_this_session_edited(repo, tmp_path):
    edit_sprint(repo)
    (repo.store / "docs").mkdir()
    (repo.store / "docs/café — notes.md").write_text("new\n")  # non-ASCII: the reason escapes it as é
    t = transcript(tmp_path / "t.jsonl", {"type": "user", "message": {"content": "go"}},
                   tool_use({"file_path": str(repo.records / "sprints/demo-1.md")}),
                   tool_use({"command": "cat 'records/docs/café — notes.md'"}))
    code, out, _ = same(repo, "hook", "stop", event={"cwd": str(repo.root), "transcript_path": t})
    reason = json.loads(out)["reason"]
    assert code == 0 and "- records/sprints/demo-1.md\n- records/docs/café — notes.md" in reason


def test_stop_blocks_on_a_codex_call(repo, tmp_path):
    edit_sprint(repo)
    t = transcript(tmp_path / "t.jsonl", {"type": "response_item", "payload": {
        "type": "function_call", "arguments": json.dumps({"cmd": "vi records/sprints/demo-1.md"})}})
    code, out, _ = same(repo, "hook", "stop", event={"cwd": str(repo.root), "transcript_path": t})
    assert code == 0 and json.loads(out)["decision"] == "block"


@pytest.mark.parametrize("case", ["other-session", "active", "no-transcript", "unreadable", "clean", "not-json",
                                  "outside-git"])
def test_stop_lets_the_stop_through(repo, tmp_path, case):
    edit_sprint(repo)
    t = transcript(tmp_path / "t.jsonl", {"type": "user", "message": {"content": "git status: sprints/demo-1.md"}},
                   tool_use({"file_path": str(repo.records / "sprints/demo-2.md")}))
    event = {"cwd": str(repo.root), "transcript_path": t}
    if case == "active":
        event = dict(event, transcript_path=transcript(tmp_path / "a.jsonl", tool_use({"file_path": "sprints/demo-1.md"})),
                     stop_hook_active=True)
    elif case == "no-transcript":
        del event["transcript_path"]
    elif case == "unreadable":
        event["transcript_path"] = str(tmp_path / "missing.jsonl")
    elif case == "clean":
        repo.git("checkout", "--", ".", cwd=repo.store)
    elif case == "not-json":
        event = "not json"
    elif case == "outside-git":
        event["cwd"] = str(tmp_path)
    code, out, _ = same(repo, "hook", "stop", event=event)
    assert code == 0 and out == ""


# ---------------------------------------------------------------- argument errors


@pytest.mark.parametrize("args", [
    ["nope"], ["task"], ["task", "close"], ["doc", "new", "x", "--title", "t"], ["prime", "--rules", "1", "--state"],
    ["prime", "--rules", "9"], ["where", "nope"], ["finding", "add", "--sprint"], ["show", "--bogus"],
    ["prime", "--rules", "x"], ["decision", "add", "--decision", "d", "--reason", "r", "--level", "x"],
    ["prime", "--rules", "1", "--s"], ["where", "- a b"], ["where", "-5"], ["where", "-x"],
])
def test_argument_errors(repo, args):
    code, _, err = same(repo, *args)
    assert code == 2 and ": error: " in err


@pytest.mark.parametrize("args", [["--rules", "1", "--hook"], ["--rules", "01"], ["--rules", " +2", "--hook-js"],
                                  ["--subagent", "--hook"]])
def test_arguments_as_argparse_reads_them(repo, args):
    """Abbreviated options and an int given as int() reads it."""
    code, out, _ = same(repo, "prime", *args, event="")
    assert code == 0 and out


# ---------------------------------------------------------------- --help


def commands() -> list[list[str]]:
    """Every parser in Python pm's tree as its argv: pm, each noun, each command under a noun."""
    out = [[]]

    def walk(p, path):
        for a in p._actions:
            if isinstance(a, argparse._SubParsersAction):
                for name, sub in a._name_parser_map.items():
                    out.append([*path, name])
                    walk(sub, [*path, name])
    walk(parser(), [])
    return out


COMMANDS = commands()
# `pm init --help` starts nothing, but the light set's guard reads only the argv: mark what it names
HELP_CASES = [pytest.param(c, marks=[pytest.mark.integration] if integration_only(["pm", *c]) else [],
                           id=" ".join(["pm", *c])) for c in COMMANDS]


def test_the_tree_has_every_command():
    leaves = [c for c in COMMANDS if not any(d[:len(c)] == c and len(d) > len(c) for d in COMMANDS)]
    assert (len(COMMANDS), len(leaves)) == (56, 40)  # pm, 15 nouns with commands under them, 40 commands


def normalised(text: str) -> str:
    return re.sub(r"\s+", " ", text).strip()


@pytest.mark.parametrize("argv", HELP_CASES)
def test_help(repo, argv):
    py, go = run(repo, "python", *argv, "--help"), run(repo, "go", *argv, "--help")
    assert (go[0], normalised(go[1]), go[2]) == (py[0], normalised(py[1]), py[2])


# ---------------------------------------------------------------- the work store's import from bd

BD_FIXTURE = Path(__file__).resolve().parent.parent / "internal/work/testdata"


def present(v):
    """v without what is absent: None, "" and empty lists and objects are dropped at every level. Python's mapper
    gives every field (null where it does not apply); pm export leaves such a field out."""
    if isinstance(v, dict):
        return {k: present(x) for k, x in v.items() if x not in (None, "", [], {})}
    if isinstance(v, list):
        return [present(x) for x in v]
    return v


@pytest.mark.integration  # pm init, though with --import-bd Go pm only imports
def test_import_bd_agrees_with_the_python_mapper(repo):
    """Go pm's pm init --import-bd, read back with pm export, gives the items that work_items.py (the neutral tests'
    mapping of bd issues) gives for the same export. On the fixture, or on a real export with the records store it
    goes with: PM_BD_EXPORT=<bd export > file> PM_BD_RECORDS=<pm where records>."""
    export = Path(os.environ.get("PM_BD_EXPORT") or BD_FIXTURE / "bd-export.jsonl")
    records = Path(os.environ.get("PM_BD_RECORDS") or BD_FIXTURE / "records")
    for kind in ("projects", "sprints"):
        (repo.store / kind).mkdir(exist_ok=True)
        for f in (records / kind).glob("*.md"):
            shutil.copy(f, repo.store / kind / f.name)
    code, out, err = run(repo, "go", "init", "--import-bd", str(export))
    assert (code, err) == (0, ""), err
    code, out, err = run(repo, "go", "export")
    assert (code, err) == (0, ""), err
    go = {i["id"]: present(i) for i in map(json.loads, out.splitlines())}
    py = {i: present(x) for i, x in work_items(list(map(json.loads, export.read_text().splitlines()))).items()}
    assert sorted(go) == sorted(py)
    assert [i for i in py if go[i] != py[i]] == []
    code, _, err = run(repo, "go", "init", "--import-bd", str(export))
    assert code == 1 and "an import goes into an empty store only" in err
