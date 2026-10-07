"""The repo's .pm/config.toml: every pm command fails hard without it or on another pinned version."""

from __future__ import annotations

import json

import pytest

from conftest import write_config
from pm import __version__

# prime's parts share one config check; --subagent is the one that starts no pm setup (a light test may not)
COMMANDS = [("show",), ("where",), ("prime", "--subagent"), ("hook", "stop")]


def test_version_has_one_source():
    import tomllib
    from pathlib import Path
    pyproject = tomllib.loads((Path(__file__).resolve().parents[1] / "pyproject.toml").read_text())
    assert __version__ == pyproject["project"]["version"]


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
        f"error: this repo pins pm 9.9.9 in {path.resolve()}, but pm {__version__} is running, launched for that "
        f"pin: release tag pm-v9.9.9 at https://github.com/Yeeef/yeeef-agents builds pm {__version__}; fix the tag, "
        f"or move the pin to {__version__} with pm upgrade\n")


def help_texts(ap) -> list[str]:
    """`ap`'s --help and that of every command under it, as argparse prints them."""
    import argparse
    out = [ap.format_help()]
    for action in ap._actions:
        if isinstance(action, argparse._SubParsersAction):
            for sub in action.choices.values():
                out += help_texts(sub)
    return out


def test_every_config_key_is_named_in_some_commands_help():
    """pm explains its own config: each key config.KEYS accepts is named, as a word, in the --help of some command
    that also names .pm/config.toml, so nobody reads pm's source to learn what a key does or which command sets it."""
    import re
    from pm import config
    from pm.cli import parser
    texts = [t for t in help_texts(parser()) if config.REL in t]
    missing = [k for k in config.KEYS if not any(re.search(rf"(?<![\w-]){k}(?![\w-])", t) for t in texts)]
    assert not missing, f"{config.REL} keys no --help names: {', '.join(missing)}"
