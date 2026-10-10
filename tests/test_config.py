"""The repo's .pm/config.toml: every pm command fails hard without it or on another pinned version, and a pin move
names the worktrees whose branches still pin the old version."""

from __future__ import annotations

import json
import re
import subprocess
from pathlib import Path

import pytest

from conftest import PM, PM_BEFORE, VERSION, VERSION_BEFORE, write_config

# prime's parts share one config check; --subagent is the one that starts no pm setup (a light test may not)
COMMANDS = [("show",), ("where",), ("prime", "--subagent"), ("hook", "stop")]
CONFIG = ".pm/config.toml"
KEYS = ("version", "remote", "main_branch", "port", "site_url")  # what the config accepts


@pytest.mark.parametrize("args", COMMANDS)
def test_every_command_fails_without_a_config(repo, args):
    (repo.root / ".pm/config.toml").unlink()
    res = repo.pm(*args, stdin=json.dumps({"cwd": str(repo.root)}))
    assert res.returncode == 1 and res.stdout == ""
    assert res.stderr == (f"error: this repo has no .pm/config.toml (looked for {repo.root.resolve()}/.pm/config.toml); "
                          "create it with pm init\n")


@pytest.mark.parametrize("args", COMMANDS)
def test_every_command_fails_on_another_pinned_version(repo, args):
    """Once launched for the pin (test_launch.py has the launch), a pm on another version fails hard."""
    path = write_config(repo.root, version="9.9.9")
    repo.env = dict(repo.env, PM_LAUNCHED="9.9.9")
    res = repo.pm(*args, stdin=json.dumps({"cwd": str(repo.root)}))
    assert res.returncode == 1 and res.stdout == ""
    assert res.stderr == (
        f"error: this repo pins pm 9.9.9 in {path.resolve()}, but pm {VERSION} is running, launched for that "
        f"pin: release tag pm-v9.9.9 at https://github.com/Yeeef/pm builds pm {VERSION}; fix the tag, "
        f"or move the pin to {VERSION} with pm upgrade --to {VERSION}\n")


def help_texts(*cmd: str) -> list[str]:
    """pm <cmd> --help and that of every command under it."""
    text = subprocess.run([*PM, *cmd, "--help"], cwd="/", check=True, capture_output=True, text=True).stdout
    subs = re.search(r"\{([a-z,-]+)\} \.\.\.", text.split("\n\n", 1)[0])  # the usage's commands, not a choice
    return [text, *(t for sub in (subs.group(1).split(",") if subs else []) for t in help_texts(*cmd, sub))]


def test_every_config_key_is_named_in_some_commands_help():
    """pm explains its own config: each key the config accepts is named, as a word, in the --help of some command
    that also names .pm/config.toml, so nobody reads pm's source to learn what a key does or which command sets it."""
    texts = [t for t in help_texts() if CONFIG in t]
    missing = [k for k in KEYS if not any(re.search(rf"(?<![\w-]){k}(?![\w-])", t) for t in texts)]
    assert texts and not missing, f"{CONFIG} keys no --help names: {', '.join(missing)}"


@pytest.mark.integration  # pm upgrade and pm doctor
def test_a_pin_move_names_the_worktrees_it_strands(repo):
    """Main pins VERSION_BEFORE; three worktrees are cut from it, and one moves the pin to VERSION. pm upgrade there
    lists the other two, whose branches still pin VERSION_BEFORE. Once the move is on main and the service runs
    VERSION, a worktree rebased onto the move drops off the list, one that moves the pin further is listed with its own
    fix, and in the one that is not rebased, the pm its branch pins names the mismatch and the fix in pm where, pm
    doctor and any work-store command."""
    cfg = repo.root / ".pm/config.toml"
    repo.stop_service()  # the service follows main's pin: it runs VERSION again once the move is on main
    cfg.write_text(cfg.read_text().replace(f'version = "{VERSION}"', f'version = "{VERSION_BEFORE}"'))
    repo.git("commit", "-qm", "pm before the move", "--", ".pm/config.toml")
    trees = {}
    for name in ("stale", "fresh", "move"):
        trees[name] = repo.root.parent / name
        repo.git("worktree", "add", "-q", "-b", name, str(trees[name]))

    def stranded(out: str) -> tuple[str, dict[Path, str]]:
        """The head line of pm upgrade's list of stranded worktrees, and its entries by path."""
        lines = out.splitlines()
        at = next(i for i, l in enumerate(lines) if "pm refuses every work-store command in these worktrees" in l)
        entries = {}
        for line in lines[at + 1:]:
            path, rest = line.strip().split("  ", 1)
            entries[Path(path).resolve()] = rest
        return lines[at], entries

    res = repo.pm("upgrade", "--to", VERSION, cwd=trees["move"])
    assert res.returncode == 0, res.stderr
    assert f"moved the pin from {VERSION_BEFORE} to {VERSION}" in res.stdout, res.stdout
    head, entries = stranded(res.stdout)
    assert head == (f"once the pin move to pm {VERSION} is on main and the main checkout pulls it, the pm service runs "
                    "it, and pm refuses every work-store command in these worktrees, whose branches pin another pm:")
    fix = f"pins pm {VERSION_BEFORE}: merge or rebase it onto the pin move"
    assert entries == {trees["stale"].resolve(): f"branch stale {fix}", trees["fresh"].resolve(): f"branch fresh {fix}"}

    repo.git("commit", "-qam", f"Upgrade pm to {VERSION}", cwd=trees["move"])
    repo.git("merge", "-q", "--ff-only", "move")  # the move reaches main and the main checkout
    repo.start_service()
    repo.git("rebase", "-q", "main", cwd=trees["fresh"])
    ahead = repo.worktree("ahead")  # a branch that moves the pin further: rebasing would not help it
    (ahead / ".pm/config.toml").write_text(
        (ahead / ".pm/config.toml").read_text().replace(f'version = "{VERSION}"', 'version = "9.9.9"'))
    repo.git("commit", "-qam", "pm 9.9.9", cwd=ahead)

    res = repo.pm("upgrade", "--to", VERSION, cwd=trees["move"])
    assert res.returncode == 0, res.stderr
    assert f"pm {VERSION}: every managed piece is current; nothing to commit" in res.stdout, res.stdout
    head, entries = stranded(res.stdout)
    assert head == (f"the main checkout pins pm {VERSION}, so the pm service runs it, and pm refuses every "
                    "work-store command in these worktrees, whose branches pin another pm:")
    assert entries == {trees["stale"].resolve(): f"branch stale {fix}", ahead.resolve():
                       "branch ahead pins pm 9.9.9: it moves the pin further ahead, and pm runs there once that move "
                       "is on main"}

    def before(*args: str) -> subprocess.CompletedProcess:
        """pm in the stale worktree, as its launcher runs it there: the pm its branch pins."""
        return subprocess.run([*PM_BEFORE, *args], cwd=trees["stale"], env=repo.env, capture_output=True, text=True,
                              stdin=subprocess.DEVNULL)

    mismatch = (f"this checkout pins pm {VERSION_BEFORE}, but the clone's pm service runs pm {VERSION}, which "
                f"{repo.root.resolve() / '.pm/config.toml'} pins: this branch is from before main moved the pin to pm "
                f"{VERSION}; merge or rebase it onto that pin move (git rebase origin/main), then pm runs pm {VERSION} "
                "here")
    res = before("where")
    assert res.returncode == 0, res.stderr
    work = next(l for l in res.stdout.splitlines() if l.startswith("work "))
    assert work.endswith("  " + mismatch), work
    res = before("doctor")
    assert res.returncode == 1 and "work store: " + mismatch in res.stdout.splitlines(), res.stdout
    res = before("show")
    assert res.returncode == 1 and res.stderr == f"error: {mismatch}\n", res.stderr
    res = repo.pm("where", cwd=trees["fresh"])
    work = next(l for l in res.stdout.splitlines() if l.startswith("work "))
    assert "pins pm" not in work and repo.pm("show", cwd=trees["fresh"]).returncode == 0, work
