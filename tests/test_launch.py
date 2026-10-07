"""The launcher (pm/launch.py): the pm uv tool runs each repo's pinned version through `uv tool run`. A fake `uv`
first on PATH logs each call with the markers it got; a fake `git` answers `ls-remote` and passes every other call to
git; the integration test runs a real older release, built by real uv from this clone's own tag."""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
from pathlib import Path

import pytest

from conftest import write_config
from pm import __version__, config, launch

FAKE_UV = '''#!{py}
import json, os, sys
stdin = "" if "--help" in sys.argv else sys.stdin.read()
with open(os.environ["FAKE_UV_LOG"], "a") as f:
    f.write(json.dumps({{"argv": sys.argv[1:], "stdin": stdin, "launched": os.environ.get("PM_LAUNCHED"),
                        "launcher": os.environ.get("PM_LAUNCHER"), "tool_dir": os.environ.get("UV_TOOL_DIR"),
                        "path0": os.environ["PATH"].split(os.pathsep)[0]}}) + "\\n")
if "--help" in sys.argv and os.environ.get("FAKE_UV_FAIL"):
    print("error: Git operation failed", file=sys.stderr)
    sys.exit(2)
print("the launched pm ran")
sys.exit(int(os.environ.get("FAKE_UV_EXIT", "0")))
'''

FAKE_GIT = '''#!{py}
import os, sys
if sys.argv[1:2] == ["ls-remote"]:
    with open(os.environ["FAKE_UV_LOG"], "a") as f:
        f.write('{{"git": "ls-remote"}}\\n')
    sys.stdout.write(os.environ.get("FAKE_LS_REMOTE", ""))
    sys.exit(0)
os.execv({git!r}, ["git", *sys.argv[1:]])
'''

SHA = "0123456789abcdef0123456789abcdef01234567"
REQ = f"git+{config.REPO}@{SHA}#subdirectory=pm"


@pytest.fixture
def fakes(repo, tmp_path):
    """The repo's env with the fake uv and git first on PATH and the pins' data dir under tmp."""
    bindir = tmp_path / "launchbin"
    bindir.mkdir()
    (bindir / "uv").write_text(FAKE_UV.format(py=sys.executable))
    (bindir / "git").write_text(FAKE_GIT.format(py=sys.executable, git=shutil.which("git")))
    for f in bindir.iterdir():
        f.chmod(0o755)
    repo.env = dict(repo.env, PATH=f"{bindir}{os.pathsep}{repo.env['PATH']}", FAKE_UV_LOG=str(tmp_path / "launch.log"),
                    XDG_DATA_HOME=str(tmp_path / "data"))
    return repo


def calls(repo) -> list[dict]:
    path = Path(repo.env["FAKE_UV_LOG"])
    return [json.loads(l) for l in path.read_text().splitlines()] if path.exists() else []


def keep(repo, version: str, sha: str = SHA) -> Path:
    path = Path(repo.env["XDG_DATA_HOME"]) / "pm/pins" / version / "commit"
    path.parent.mkdir(parents=True)
    path.write_text(sha + "\n")
    return path


def test_a_repo_pinned_to_this_version_runs_in_process_with_no_uv_call(fakes):
    res = fakes.pm("where")
    assert res.returncode == 0, res.stderr
    assert res.stdout.splitlines()[0] == f"pm        {__version__}  run in process: it is this repo's pin, or the repo pins none yet"
    assert calls(fakes) == []


def test_another_pin_execs_uv_with_the_pins_commit_and_passes_stdin_stdout_and_the_exit_code(fakes):
    write_config(fakes.root, version="9.9.9")
    keep(fakes, "9.9.9")
    fakes.env = dict(fakes.env, FAKE_UV_EXIT="7")
    res = fakes.pm("hook", "stop", stdin='{"cwd": "x"}')
    assert (res.returncode, res.stdout, res.stderr) == (7, "the launched pm ran\n", "")
    assert calls(fakes) == [{"argv": ["--quiet", "tool", "run", "--from", REQ, "pm", "hook", "stop"],
                             "stdin": '{"cwd": "x"}', "launched": "9.9.9", "launcher": __version__,
                             "tool_dir": fakes.env["UV_TOOL_DIR"], "path0": fakes.env["PATH"].split(os.pathsep)[0]}]


def test_the_first_launch_of_a_pin_resolves_its_tag_builds_it_once_then_keeps_the_commit(fakes):
    write_config(fakes.root, version="9.9.9")
    env = dict(fakes.env, FAKE_LS_REMOTE=f"aaaa\trefs/tags/pm-v9.9.9\n{SHA}\trefs/tags/pm-v9.9.9^{{}}\n")
    fakes.env = env
    assert fakes.pm("show").returncode == 0
    assert fakes.pm("show").returncode == 0
    log = calls(fakes)
    assert log[0] == {"git": "ls-remote"}
    assert log[1]["argv"] == ["tool", "run", "--from", REQ, "pm", "--help"]
    assert [c["argv"] for c in log[2:]] == [["--quiet", "tool", "run", "--from", REQ, "pm", "show"]] * 2
    assert (Path(env["XDG_DATA_HOME"]) / "pm/pins/9.9.9/commit").read_text() == SHA + "\n"


def test_a_pin_whose_release_cannot_be_fetched_fails_hard_naming_the_tag_and_the_command(fakes):
    write_config(fakes.root, version="9.9.9")
    fakes.env = dict(fakes.env, FAKE_LS_REMOTE=f"{SHA}\trefs/tags/pm-v9.9.9\n", FAKE_UV_FAIL="1")
    res = fakes.pm("show")
    assert res.returncode == 1 and res.stdout == ""
    assert res.stderr == (
        f"error: this repo pins pm 9.9.9, which the pm uv tool (pm {__version__}) runs through uv, but uv could not "
        f"fetch and build pm-v9.9.9 ({SHA}): error: Git operation failed; check the network and that tag pm-v9.9.9 "
        "exists, then run pm again; or run it yourself with uv tool run --from "
        f'"git+{config.REPO}@pm-v9.9.9#subdirectory=pm" pm …, or move the pin with pm upgrade\n')
    assert not (Path(fakes.env["XDG_DATA_HOME"]) / "pm/pins/9.9.9/commit").exists()
    assert [c.get("argv", [""])[-1] for c in calls(fakes)] == ["", "--help"]  # ls-remote, the build; no launch


def test_a_pin_with_no_release_tag_fails_hard(fakes):
    write_config(fakes.root, version="9.9.9")
    res = fakes.pm("show")
    assert res.returncode == 1
    assert res.stderr.startswith(f"error: this repo pins pm 9.9.9, which the pm uv tool (pm {__version__}) runs through "
                                 f"uv, but release tag pm-v9.9.9 was not found at {config.REPO} (no such tag); ")
    assert calls(fakes) == [{"git": "ls-remote"}]


def test_a_pm_launched_for_its_pin_that_runs_another_version_fails_instead_of_launching_again(fakes):
    path = write_config(fakes.root, version="9.9.9")
    fakes.env = dict(fakes.env, PM_LAUNCHED="9.9.9")
    res = fakes.pm("show")
    assert res.returncode == 1 and calls(fakes) == []
    assert res.stderr == (f"error: this repo pins pm 9.9.9 in {path.resolve()}, but pm {__version__} is running, "
                          f"launched for that pin: release tag pm-v9.9.9 at {config.REPO} builds pm {__version__}; "
                          f"fix the tag, or move the pin to {__version__} with pm upgrade\n")


def test_a_pm_launched_for_another_repos_pin_still_launches_this_repos_pin(fakes):
    write_config(fakes.root, version="9.9.9")
    keep(fakes, "9.9.9")
    fakes.env = dict(fakes.env, PM_LAUNCHED="8.8.8")
    assert fakes.pm("show").returncode == 0
    assert calls(fakes)[0]["launched"] == "9.9.9"


def test_a_pin_older_than_the_launcher_keeps_its_own_uv_tool_dirs(fakes):
    write_config(fakes.root, version="0.1.0")
    keep(fakes, "0.1.0")
    assert fakes.pm("show").returncode == 0
    pins = Path(fakes.env["XDG_DATA_HOME"]) / "pm/pins/0.1.0"
    (call,) = calls(fakes)
    assert (call["tool_dir"], call["path0"]) == (str(pins / "tools"), str(pins / "bin"))


def test_upgrade_runs_in_process_unless_it_names_another_version(tmp_path):
    write_config(tmp_path, version="9.9.9")
    subprocess.run(["git", "init", "-q", str(tmp_path)], check=True)
    assert launch.target(["show"], tmp_path, {}) == "9.9.9"
    assert launch.target(["upgrade"], tmp_path, {}) is None
    assert launch.target(["upgrade", "--to", "8.8.8"], tmp_path, {}) == "8.8.8"
    assert launch.target(["upgrade", "--to=8.8.8"], tmp_path, {}) == "8.8.8"
    assert launch.target(["upgrade", "--to", __version__], tmp_path, {}) is None
    assert launch.target(["show"], tmp_path / "nowhere", {}) is None  # no repo: the config check says so


def git_common_dir() -> str:
    return subprocess.run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"], capture_output=True,
                          text=True, check=True, cwd=Path(__file__).parent).stdout.strip()


REWRITE = '''#!{py}
import os, sys
args = [a.replace({repo!r}, {local!r}) for a in sys.argv[1:]]
os.execv({real!r}, [{name!r}, *args])
'''


@pytest.mark.integration
def test_the_new_tool_runs_pm_0_1_0_in_a_repo_pinned_to_it(repo, tmp_path):
    """Two built versions: this checkout's pm launches release 0.1.0, which real uv builds from this clone's tag."""
    local = f"file://{git_common_dir()}"
    if subprocess.run(["git", "rev-parse", "-q", "--verify", "refs/tags/pm-v0.1.0"], cwd=Path(__file__).parent,
                      capture_output=True).returncode != 0:
        pytest.fail("this clone has no tag pm-v0.1.0; git fetch --tags")
    bindir = tmp_path / "rewrite"
    bindir.mkdir()
    for name in ("uv", "git"):  # the release URL points at this clone, so nothing reaches GitHub
        (bindir / name).write_text(REWRITE.format(py=sys.executable, repo=config.REPO, local=local,
                                                  real=shutil.which(name), name=name))
        (bindir / name).chmod(0o755)
    write_config(repo.root, version="0.1.0")
    repo.commit("pin 0.1.0")
    env = {k: v for k, v in repo.env.items() if k != "PYTHONPATH"}  # the fake tool's dist-info would shadow 0.1.0's
    repo.env = dict(env, PATH=f"{bindir}{os.pathsep}{env['PATH']}", XDG_DATA_HOME=str(tmp_path / "data"))
    res = repo.pm("show")
    assert res.returncode == 0, res.stderr
    kept = (tmp_path / "data/pm/pins/0.1.0/commit").read_text().strip()
    assert kept == subprocess.run(["git", "rev-parse", "pm-v0.1.0^{commit}"], cwd=Path(__file__).parent,
                                  capture_output=True, text=True, check=True).stdout.strip()
    where = repo.pm("where")
    assert where.returncode == 0, where.stderr
    assert not where.stdout.startswith("pm        ")  # 0.1.0's pm where, which names no version line
