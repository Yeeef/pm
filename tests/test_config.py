"""The repo's .pm/config.toml: every pm command fails hard without it or on another pinned version."""

from __future__ import annotations

import json

import pytest

from conftest import write_config
from pm import __version__

COMMANDS = [("show",), ("where",), ("prime",), ("hook", "stop")]


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
    path = write_config(repo.root, version="9.9.9")
    res = repo.pm(*args, stdin=json.dumps({"cwd": str(repo.root)}))
    assert res.returncode == 1 and res.stdout == ""
    assert res.stderr == (
        f"error: this repo pins pm 9.9.9 in {path.resolve()}, but pm {__version__} is running; install the pinned "
        'version with uv tool install "git+https://github.com/Yeeef/yeeef-agents@pm-v9.9.9#subdirectory=pm" '
        "(or move the pin with pm upgrade once it exists)\n")
