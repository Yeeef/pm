"""The launcher (pm/launch.py): the pm uv tool runs each repo's pinned version through `uv tool run`. A fake `uv`
first on PATH logs each call with the markers it got; a fake `git` answers `ls-remote` and passes every other call to
git; the integration test runs a real older release, built by real uv from this clone's own tag."""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
from pathlib import Path

import pytest

from conftest import PM, fake_bd_env, write_config
from pm import __version__, config, install, launch

IN_PROCESS = pytest.mark.impl("python", reason="runs Python pm's launcher or CLI in process")

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
    write_config(fakes.root, version="0.1.99")
    keep(fakes, "0.1.99")
    fakes.env = dict(fakes.env, FAKE_UV_EXIT="7")
    res = fakes.pm("hook", "stop", stdin='{"cwd": "x"}')
    assert (res.returncode, res.stdout, res.stderr) == (7, "the launched pm ran\n", "")
    assert calls(fakes) == [{"argv": ["--quiet", "tool", "run", "--from", REQ, "pm", "hook", "stop"],
                             "stdin": '{"cwd": "x"}', "launched": "0.1.99", "launcher": __version__,
                             "tool_dir": fakes.env["UV_TOOL_DIR"], "path0": fakes.env["PATH"].split(os.pathsep)[0]}]


def test_a_launched_text_file_body_reaches_the_pinned_pm_unread_by_the_launcher(fakes):
    write_config(fakes.root, version="0.1.99")
    keep(fakes, "0.1.99")
    args = ["feedback", "add", "--project", "demo", "--text-file", "-"]
    res = fakes.pm(*args, stdin="line one\n`code` $x\n")
    assert res.returncode == 0, res.stderr
    assert [(c["argv"][-len(args):], c["stdin"]) for c in calls(fakes)] == [(args, "line one\n`code` $x\n")]


def test_the_first_launch_of_a_pin_resolves_its_tag_builds_it_once_then_keeps_the_commit(fakes):
    write_config(fakes.root, version="0.1.99")
    env = dict(fakes.env, FAKE_LS_REMOTE=f"aaaa\trefs/tags/pm-v0.1.99\n{SHA}\trefs/tags/pm-v0.1.99^{{}}\n")
    fakes.env = env
    assert fakes.pm("show").returncode == 0
    assert fakes.pm("show").returncode == 0
    log = calls(fakes)
    assert log[0] == {"git": "ls-remote"}
    assert log[1]["argv"] == ["tool", "run", "--from", REQ, "pm", "--help"]
    assert [c["argv"] for c in log[2:]] == [["--quiet", "tool", "run", "--from", REQ, "pm", "show"]] * 2
    assert (Path(env["XDG_DATA_HOME"]) / "pm/pins/0.1.99/commit").read_text() == SHA + "\n"


def test_a_pin_whose_release_cannot_be_fetched_fails_hard_naming_the_tag_and_the_command(fakes):
    write_config(fakes.root, version="0.1.99")
    fakes.env = dict(fakes.env, FAKE_LS_REMOTE=f"{SHA}\trefs/tags/pm-v0.1.99\n", FAKE_UV_FAIL="1")
    res = fakes.pm("show")
    assert res.returncode == 1 and res.stdout == ""
    assert res.stderr == (
        f"error: this repo pins pm 0.1.99, which the pm uv tool (pm {__version__}) runs through uv, but uv could not "
        f"fetch and build pm-v0.1.99 ({SHA}): error: Git operation failed; check the network and that tag pm-v0.1.99 "
        "exists, then run pm again; or run it yourself with uv tool run --from "
        f'"git+{config.REPO}@pm-v0.1.99#subdirectory=pm" pm …, or move the pin with pm upgrade\n')
    assert not (Path(fakes.env["XDG_DATA_HOME"]) / "pm/pins/0.1.99/commit").exists()
    assert [c.get("argv", [""])[-1] for c in calls(fakes)] == ["", "--help"]  # ls-remote, the build; no launch


def test_a_pin_with_no_release_tag_fails_hard(fakes):
    write_config(fakes.root, version="0.1.99")
    res = fakes.pm("show")
    assert res.returncode == 1
    assert res.stderr.startswith(f"error: this repo pins pm 0.1.99, which the pm uv tool (pm {__version__}) runs through "
                                 f"uv, but release tag pm-v0.1.99 was not found at {config.REPO} (no such tag); ")
    assert calls(fakes) == [{"git": "ls-remote"}]


def test_a_pm_launched_for_its_pin_that_runs_another_version_fails_instead_of_launching_again(fakes):
    path = write_config(fakes.root, version="0.1.99")
    fakes.env = dict(fakes.env, PM_LAUNCHED="0.1.99")
    res = fakes.pm("show")
    assert res.returncode == 1 and calls(fakes) == []
    assert res.stderr == (f"error: this repo pins pm 0.1.99 in {path.resolve()}, but pm {__version__} is running, "
                          f"launched for that pin: release tag pm-v0.1.99 at {config.REPO} builds pm {__version__}; "
                          f"fix the tag, or move the pin to {__version__} with pm upgrade --to {__version__}\n")


def test_a_pm_launched_for_another_repos_pin_still_launches_this_repos_pin(fakes):
    write_config(fakes.root, version="0.1.99")
    keep(fakes, "0.1.99")
    fakes.env = dict(fakes.env, PM_LAUNCHED="0.1.88")
    assert fakes.pm("show").returncode == 0
    assert calls(fakes)[0]["launched"] == "0.1.99"


def test_a_pin_older_than_the_launcher_keeps_its_own_uv_tool_dirs_and_gets_no_markers(fakes):
    """0.1.0 never reads the markers and never launches, and its children would inherit them."""
    write_config(fakes.root, version="0.1.0")
    keep(fakes, "0.1.0")
    assert fakes.pm("show").returncode == 0
    pins = Path(fakes.env["XDG_DATA_HOME"]) / "pm/pins/0.1.0"
    (call,) = calls(fakes)
    assert (call["tool_dir"], call["path0"]) == (str(pins / "tools"), str(pins / "bin"))
    assert (call["launched"], call["launcher"]) == (None, None)


def test_a_pm_run_by_an_old_pin_reaches_the_launcher_without_its_tool_dirs(fakes):
    """A child of 0.1.0 whose pin bin dir holds no pm yet reaches the launcher with 0.1.0's uv tool dirs: they go,
    so another repo's pin runs with the machine's."""
    write_config(fakes.root, version="0.1.99")
    keep(fakes, "0.1.99")
    pins = Path(fakes.env["XDG_DATA_HOME"]) / "pm/pins/0.1.0"
    (pins / "bin").mkdir(parents=True)  # empty until 0.1.0's pm init installs its tool there
    env = fakes.env
    fakes.env = dict(env, UV_TOOL_DIR=str(pins / "tools"), UV_TOOL_BIN_DIR=str(pins / "bin"),
                     PATH=f"{pins / 'bin'}{os.pathsep}{env['PATH']}")
    assert fakes.pm("show").returncode == 0
    (call,) = calls(fakes)
    assert (call["launched"], call["tool_dir"], call["path0"]) == ("0.1.99", None, env["PATH"].split(os.pathsep)[0])


@IN_PROCESS
def test_a_launched_pms_children_get_no_markers_so_a_pm_they_run_launches_the_pin(fakes, monkeypatch):
    """The launched pm 0.1.99 takes the markers out of its environment; a `pm` its git hooks or `claude -p` run in
    the same repo reaches the launcher, which launches 0.1.99 again instead of running itself at its own version."""
    write_config(fakes.root, version="0.1.99")
    keep(fakes, "0.1.99")
    for k, v in fakes.env.items():
        monkeypatch.setenv(k, v)
    monkeypatch.setenv(launch.LAUNCHED, "0.1.99")
    monkeypatch.setenv(launch.LAUNCHER, __version__)
    monkeypatch.chdir(fakes.root)
    monkeypatch.setattr(launch, "__version__", "0.1.99")  # this process stands in for the launched pm 0.1.99
    monkeypatch.setattr(launch, "marks", {})
    assert launch.launch(["show"]) is None and launch.launched()
    assert launch.LAUNCHED not in os.environ and launch.LAUNCHER not in os.environ
    assert launch.how().startswith(f"this repo's pin at commit {SHA}, launched by the pm uv tool (pm {__version__}); "
                                   f"delete {Path(fakes.env['XDG_DATA_HOME']) / 'pm/pins/0.1.99/commit'}")
    child = subprocess.run([*PM, "show"], cwd=fakes.root, capture_output=True, text=True)  # inherits os.environ
    assert child.returncode == 0, child.stderr
    assert [c["launched"] for c in calls(fakes)] == ["0.1.99"]


def test_a_kept_commit_file_without_a_sha_is_resolved_again(fakes):
    write_config(fakes.root, version="0.1.99")
    path = keep(fakes, "0.1.99", sha="0123")  # cut short by a crash, say
    fakes.env = dict(fakes.env, FAKE_LS_REMOTE=f"{SHA}\trefs/tags/pm-v0.1.99\n")
    assert fakes.pm("show").returncode == 0
    assert calls(fakes)[0] == {"git": "ls-remote"} and path.read_text() == SHA + "\n"
    assert [p.name for p in path.parent.iterdir()] == ["commit"], "the temp file went"


@IN_PROCESS
@pytest.mark.parametrize("slow", ["git", "uv"])
def test_a_tag_that_does_not_resolve_or_build_in_time_fails_hard(fakes, monkeypatch, slow):
    """git ls-remote and the first build run with a timeout, and git never prompts for credentials."""
    monkeypatch.setenv("XDG_DATA_HOME", fakes.env["XDG_DATA_HOME"])
    seen = []

    def run(cmd, **kw):
        seen.append((cmd[0], kw.get("timeout"), (kw.get("env") or {}).get("GIT_TERMINAL_PROMPT")))
        if cmd[0] == slow:
            raise subprocess.TimeoutExpired(cmd, kw["timeout"])
        return subprocess.CompletedProcess(cmd, 0, f"{SHA}\trefs/tags/pm-v0.1.99\n", "")
    monkeypatch.setattr(launch.subprocess, "run", run)
    want = {"git": f"git ls-remote {config.REPO} did not answer within 10 s for release tag pm-v0.1.99; check the network",
            "uv": f"uv did not fetch and build pm-v0.1.99 ({SHA}) within 300 s; check the network"}[slow]
    with pytest.raises(launch.LaunchError, match=re.escape(want)):
        launch.commit("0.1.99", {})
    assert seen[0] == ("git", launch.RESOLVE_TIMEOUT, "0")
    assert seen[1:] == ([("uv", launch.BUILD_TIMEOUT, None)] if slow == "uv" else [])
    assert launch.kept("0.1.99") is None


@IN_PROCESS
def test_upgrade_without_to_never_moves_a_newer_pin_down(fakes, monkeypatch, capsys):
    """A bare pm upgrade runs at the pm uv tool's version; in a repo pinned newer it refuses, naming what works."""
    path = write_config(fakes.root, version="0.1.99")
    before = path.read_text()
    monkeypatch.chdir(fakes.root)
    for k, v in fakes.env.items():
        monkeypatch.setenv(k, v)
    from pm.cli import main
    assert main(["upgrade"]) == 1
    assert capsys.readouterr().err == (
        f"error: this repo pins pm 0.1.99, newer than the running pm {__version__}, and pm upgrade moves a pin down only "
        "when --to names the version; run pm upgrade --to 0.1.99 to rewrite pm's pieces at the pin, or install the "
        f'latest pm uv tool with uv tool install --reinstall "git+{config.REPO}#subdirectory=pm", then pm upgrade\n')
    assert path.read_text() == before and calls(fakes) == []


@IN_PROCESS
def test_upgrade_runs_in_process_unless_it_names_another_version(tmp_path):
    write_config(tmp_path, version="0.1.99")
    subprocess.run(["git", "init", "-q", str(tmp_path)], check=True)
    assert launch.target(["show"], tmp_path, {}) == "0.1.99"
    assert launch.target(["upgrade"], tmp_path, {}) is None
    assert launch.target(["upgrade", "--to", "0.1.88"], tmp_path, {}) == "0.1.88"
    assert launch.target(["upgrade", "--to=0.1.88"], tmp_path, {}) == "0.1.88"
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


@pytest.mark.integration
@pytest.mark.impl("python", reason="the launcher is Python pm's")
def test_a_release_tags_before_it_pins_so_both_commits_pass_the_hook(tmp_path):
    """pm/AGENTS.md, Releasing pm: commit A sets the package version and keeps the old pin, so the pm uv tool runs the
    pre-commit hook in process; with tag pm-v<new> on A in origin, commit B moves the pin and the hook launches A's pm.
    Commit B without the tag is refused, so the hook does run the launcher. Real git hooks and real uv; the release
    URL is rewritten to a scratch origin, so nothing reaches GitHub."""
    old = __version__
    new = ".".join([*old.split(".")[:-1], str(int(old.split(".")[-1]) + 1)])
    origin, work = tmp_path / "origin.git", tmp_path / "work"
    run = lambda *a, cwd=work, **kw: subprocess.run(a, cwd=cwd, env=env, capture_output=True, text=True, **kw)
    bindir = tmp_path / "rewrite"
    bindir.mkdir()
    (bindir / "uv").write_text(REWRITE.format(py=sys.executable, repo=config.REPO, local=f"file://{origin}",
                                              real=shutil.which("uv"), name="uv"))
    (bindir / "uv").chmod(0o755)
    (bindir / "pm").symlink_to(PM[0])  # the pm uv tool, at the old version: this checkout's pm
    env = {k: v for k, v in fake_bd_env(tmp_path, os.environ).items() if k != "PYTHONPATH"}  # it would shadow A's pm
    # git runs a hook with its exec dir first on PATH, so the launcher's git ls-remote is rewritten by git's config
    env.update(PATH=f"{bindir}{os.pathsep}{env['PATH']}", GIT_CONFIG_COUNT="1",
               GIT_CONFIG_KEY_0=f"url.file://{origin}.insteadOf", GIT_CONFIG_VALUE_0=config.REPO)
    kept = Path(env["XDG_DATA_HOME"]) / "pm/pins" / new / "commit"

    subprocess.run(["git", "init", "-q", "--bare", "-b", "main", str(origin)], check=True)
    run("git", "clone", "-q", str(origin), str(work), cwd=tmp_path, check=True)
    for k, v in (("user.email", "t@example.com"), ("user.name", "t"), ("core.hooksPath", ".beads/hooks")):
        run("git", "config", k, v, check=True)
    src = Path(__file__).resolve().parents[1]  # the package as this checkout has it, at the old version
    for rel in ["pyproject.toml", *(p.relative_to(src).as_posix() for p in (src / "src/pm").rglob("*")
                                    if p.is_file() and "__pycache__" not in p.parts)]:
        (work / "pm" / rel).parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(src / rel, work / "pm" / rel)
    write_config(work, version=old)
    hook = work / ".beads/hooks/pre-commit"  # pm's section, as pm init writes it into a new hook file
    hook.parent.mkdir(parents=True)
    hook.write_text(install.SHEBANG + install.git_hook_section("pre-commit"))
    hook.chmod(0o755)
    run("git", "add", "-A", check=True)
    assert run("git", "commit", "-qm", f"pm {old}").returncode == 0
    run("git", "push", "-q", "origin", "main", check=True)

    # commit A: the package version only; the pin stays at the old version
    run("git", "checkout", "-qb", "release", check=True)
    pyproject = work / "pm/pyproject.toml"
    pyproject.write_text(pyproject.read_text().replace(f'version = "{old}"', f'version = "{new}"', 1))
    run("git", "add", "-A", check=True)
    assert run("git", "ls-remote", str(origin), f"refs/tags/pm-v{new}").stdout == ""
    res = run("git", "commit", "-qm", f"pm {new}: the package version")
    assert res.returncode == 0, res.stderr
    a = run("git", "rev-parse", "HEAD", check=True).stdout.strip()
    assert not kept.exists()  # the old pm ran the hook in process

    # commit B: the pin moves; without the tag the launcher cannot run the new pm, and the hook refuses the commit
    write_config(work, version=new)
    run("git", "add", "-A", check=True)
    res = run("git", "commit", "-qm", f"pm {new}: pin it")
    assert res.returncode != 0
    assert f"release tag pm-v{new} was not found at {config.REPO} (no such tag)" in res.stderr, res.stderr
    assert run("git", "rev-parse", "HEAD", check=True).stdout.strip() == a

    run("git", "tag", "-a", f"pm-v{new}", "-m", f"pm {new}", a, check=True)
    run("git", "push", "-q", "origin", f"pm-v{new}", check=True)
    res = run("git", "commit", "-qm", f"pm {new}: pin it")
    assert res.returncode == 0, res.stderr
    assert run("git", "rev-parse", "HEAD~1", check=True).stdout.strip() == a
    assert kept.read_text().strip() == a  # the hook ran the new pm, built from the tag's commit
