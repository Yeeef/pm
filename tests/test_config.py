"""The repo's .pm/config.toml: every pm command fails hard without it or on another pinned version, and pm takes the
remote, the main branch, the port and the site URL from it."""

from __future__ import annotations

import json

import pytest

from conftest import write_config
from pm import __version__, cli, config

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


def test_the_store_reads_the_main_checkouts_config_and_a_worktree_its_own(repo):
    wt = repo.worktree("feature-x")
    write_config(repo.root, version="9.9.9")
    assert "pins pm 9.9.9" in repo.pm("show", cwd=repo.store).stderr
    assert repo.pm("show", cwd=wt).returncode == 0  # its branch still pins the running version
    write_config(wt, version="8.8.8")
    assert "pins pm 8.8.8" in repo.pm("show", cwd=wt).stderr


@pytest.mark.parametrize("text, said", [
    ('version = "0.1.0"\n', "missing keys remote, main_branch, port"),
    ('version = "0.1.0"\nremote = "o"\nmain_branch = "m"\nport = "80"\n', "wrong types for port (want int)"),
    ('version = "0.1.0"\nremote = "o"\nmain_branch = "m"\nport = 80\nsiteUrl = "x"\n', "unknown keys siteUrl"),
    ("version = \n", "is not valid TOML"),
])
def test_a_malformed_config_fails(repo, text, said):
    (repo.root / ".pm/config.toml").write_text(text)
    res = repo.pm("show")
    assert res.returncode == 1 and said in res.stderr


def test_remote_and_main_branch_come_from_the_config(repo, monkeypatch):
    write_config(repo.root, remote="upstream", main_branch="trunk")
    res = repo.pm("where")
    assert res.returncode == 0, res.stderr
    assert ", no upstream/records" in res.stdout.splitlines()[0]
    monkeypatch.chdir(repo.root)
    config.load.cache_clear()
    assert cli.pull_main(repo.root) == f"git -C {repo.root} pull --ff-only upstream trunk"


def test_port_comes_from_the_config_and_PORT_overrides_it_for_one_run(repo):
    write_config(repo.root, port=1)  # nothing listens on port 1
    repo.env.pop("PORT", None)
    res = repo.pm("record", "link", "demo.1")
    assert res.returncode == 1
    assert "no site is served on :1; start it with pm service install, then" in res.stderr
    assert repo.pm("show").stdout.count("site: http://localhost:1 (the pm service)") == 1
    repo.env["PORT"] = "2"
    res = repo.pm("record", "link", "demo.1")
    assert "no site is served on :2; start it with pm service install, then" in res.stderr


def test_site_url_comes_from_the_config(repo):
    write_config(repo.root, site_url="https://pm.example.com/")
    res = repo.pm("show")
    assert "site: https://pm.example.com (the pm service)" in res.stdout
