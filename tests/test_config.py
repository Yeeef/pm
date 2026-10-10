"""The repo's .pm/config.toml: every pm command fails hard without it or on another pinned version."""

from __future__ import annotations

import json
import re
import subprocess

import pytest

from conftest import PM, VERSION, write_config

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
