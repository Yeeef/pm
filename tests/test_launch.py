"""The launcher (pm/launch.py): the pm uv tool runs each repo's pinned version through `uv tool run`. A fake `uv`
first on PATH logs each call with the markers it got; a fake `git` answers `ls-remote` and passes every other call to
git; the integration test runs a real older release, built by real uv from this clone's own tag. A Go pin (0.2.0 and
up) runs its release binary instead: `release` serves Go releases on 127.0.0.1 through PM_RELEASE_URL, each tarball
holding a fake pm script that prints its argv, markers and stdin; `github` serves them as GitHub's API does, for the
download with a token."""

from __future__ import annotations

import gzip
import hashlib
import http.server
import io
import json
import os
import platform
import re
import shutil
import subprocess
import sys
import tarfile
import threading
from pathlib import Path
from types import SimpleNamespace

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


# A Go release's fake pm: prints how it was run and exits with the code the release was built with.
FAKE_GO_PM = '''#!/bin/sh
echo "argv0=$0"
for a in "$@"; do echo "arg=$a"; done
echo "launched=$PM_LAUNCHED launcher=$PM_LAUNCHER"
echo "stdin=$(cat)"
exit {code}
'''
# The asset this machine runs: a Go release's only macOS binary is arm64, which an Apple silicon Mac runs even when
# the tests' Python is x86_64 under Rosetta
PLATFORM = "darwin-arm64" if sys.platform == "darwin" else f"linux-{'amd64' if platform.machine() == 'x86_64' else '?'}"
FIX = "; check the network and the release, then run pm again, or move the pin with pm upgrade"


@pytest.fixture
def release(fakes, tmp_path):
    """Go releases under `root` (one pm-v<X> dir each), served on 127.0.0.1 through PM_RELEASE_URL (given with a
    trailing slash, which the launcher strips); `requests` logs each path asked for."""
    root, requests = tmp_path / "releases", []
    root.mkdir()

    class Handler(http.server.SimpleHTTPRequestHandler):
        def __init__(self, *args, **kwargs):
            super().__init__(*args, directory=str(root), **kwargs)

        def do_GET(self):
            requests.append(self.path)
            super().do_GET()

        def log_message(self, *args):
            pass

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    url = f"http://127.0.0.1:{server.server_port}"
    fakes.env = dict(fakes.env, PM_RELEASE_URL=url + "/")
    yield SimpleNamespace(root=root, url=url, requests=requests)
    server.shutdown()
    server.server_close()


TOKEN = "gho_test_token"


class GitHub:
    """A stand-in for GitHub's API over the releases under `root`, on 127.0.0.1, as the launchers and install.sh call
    it: GET <api>/releases/tags/<tag> answers the release's assets, each with its API url (and, as GitHub's JSON does,
    other urls that name no asset); GET <api>/releases/assets/<n> with Accept application/octet-stream redirects to the
    asset's file on another host name (localhost), which, as GitHub's storage host does, refuses a request carrying a
    token. The API answers 401 without `Bearer TOKEN`. `requests` logs (path, whether it carried the token)."""

    def __init__(self, root: Path):
        self.root, self.requests, self.assets = root, [], []
        gh = self

        class Handler(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                auth = self.headers.get("Authorization")
                gh.requests.append((self.path, auth is not None))
                parts = self.path.strip("/").split("/")
                if parts[0] == "files":  # the storage host
                    path = gh.root.joinpath(*parts[1:])
                    if auth is not None:
                        return self.answer(400, b"Only one auth mechanism allowed")
                    return self.answer(200, path.read_bytes()) if path.is_file() else self.answer(404, b"")
                if auth != f"Bearer {TOKEN}":
                    return self.answer(401, b'{"message": "Bad credentials"}')
                if parts[:3] == ["api", "releases", "tags"] and (gh.root / parts[3]).is_dir():
                    return self.answer(200, json.dumps(gh.release(parts[3])).encode())
                if parts[:3] == ["api", "releases", "assets"] and self.headers.get("Accept") == "application/octet-stream":
                    tag, name = gh.assets[int(parts[3])]
                    self.send_response(302)
                    self.send_header("Location", f"http://localhost:{gh.port}/files/{tag}/{name}")
                    self.send_header("Content-Length", "0")
                    return self.end_headers()
                self.answer(404, b'{"message": "Not Found"}')

            def answer(self, code: int, body: bytes):
                self.send_response(code)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, *args):
                pass

        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.port = self.server.server_port
        self.api = f"http://127.0.0.1:{self.port}/api"
        threading.Thread(target=self.server.serve_forever, daemon=True).start()

    def release(self, tag: str) -> dict:
        assets = []
        for f in sorted((self.root / tag).iterdir()):
            self.assets.append((tag, f.name))
            assets.append({"url": f"{self.api}/releases/assets/{len(self.assets) - 1}", "id": len(self.assets) - 1,
                           "name": f.name, "uploader": {"url": "https://api.github.com/users/someone"},
                           "browser_download_url": f"https://github.com/o/r/releases/download/{tag}/{f.name}"})
        return {"url": f"{self.api}/releases/1", "assets_url": f"{self.api}/releases/1/assets", "name": f"pm {tag}",
                "assets": assets}

    def close(self):
        self.server.shutdown()
        self.server.server_close()


def tarball(path: Path, files: dict[str, bytes]) -> str:
    """Write a gzip tar holding `files`, each mode 0755; its sha256, the same on every run (gzip's mtime is 0), so
    the texts that name it compare across implementations."""
    with gzip.GzipFile(path, "wb", mtime=0) as gz, tarfile.open(fileobj=gz, mode="w") as tf:
        for name, body in files.items():
            info = tarfile.TarInfo(name)
            info.size, info.mode = len(body), 0o755
            tf.addfile(info, io.BytesIO(body))
    return hashlib.sha256(path.read_bytes()).hexdigest()


def publish(release, version: str, code: int = 0) -> tuple[Path, str]:
    """Release pm-v<version>: this platform's tarball holding a fake pm that exits `code`, and SHA256SUMS listing it
    beside another platform's; the tarball and its sha256."""
    d = release.root / f"pm-v{version}"
    d.mkdir()
    tar = d / f"pm-{version}-{PLATFORM}.tar.gz"
    sha = tarball(tar, {"pm": FAKE_GO_PM.format(code=code).encode()})
    (d / "SHA256SUMS").write_text(f"{'0' * 64}  pm-{version}-plan9-amd64.tar.gz\n{sha}  {tar.name}\n")
    return tar, sha


def go_pin(repo, version: str) -> Path:
    return Path(repo.env["XDG_DATA_HOME"]) / "pm/pins" / version


def test_a_go_pin_downloads_checks_and_keeps_its_release_binary_then_execs_it(fakes, release):
    """The first launch of a Go pin fetches SHA256SUMS and this platform's tarball, keeps the checked binary and the
    tarball's sha256, and execs the binary: its stdout, stdin and exit code are the run's, with the markers set."""
    write_config(fakes.root, version="0.2.0")
    tar, sha = publish(release, "0.2.0", code=7)
    res = fakes.pm("show", "--project", "a b", stdin="the input")
    pin = go_pin(fakes, "0.2.0")
    assert (res.returncode, res.stderr) == (7, "")
    assert res.stdout == (f"argv0={pin / 'pm'}\narg=show\narg=--project\narg=a b\n"
                          f"launched=0.2.0 launcher={__version__}\nstdin=the input\n")
    assert release.requests == ["/pm-v0.2.0/SHA256SUMS", f"/pm-v0.2.0/{tar.name}"]
    assert sorted(p.name for p in pin.iterdir()) == ["pm", "sha256"], "no temp file is left"
    assert (pin / "pm").read_text() == FAKE_GO_PM.format(code=7) and (pin / "pm").stat().st_mode & 0o777 == 0o755
    assert (pin / "sha256").read_text() == sha + "\n"
    assert calls(fakes) == [], "no uv and no git ls-remote"


def test_a_kept_go_binary_runs_with_no_request(fakes, release):
    write_config(fakes.root, version="0.2.0")
    publish(release, "0.2.0")
    assert fakes.pm("show").returncode == 0
    release.requests.clear()
    shutil.rmtree(release.root / "pm-v0.2.0")  # the release is gone; the kept binary does not need it
    res = fakes.pm("show")
    assert res.returncode == 0, res.stderr
    assert res.stdout.startswith(f"argv0={go_pin(fakes, '0.2.0') / 'pm'}\narg=show\n")
    assert release.requests == []


def test_a_pre_release_pin_is_a_go_pin(fakes, release):
    write_config(fakes.root, version="0.2.0-rc.1")
    publish(release, "0.2.0-rc.1")
    res = fakes.pm("show")
    assert res.returncode == 0, res.stderr
    assert "launched=0.2.0-rc.1" in res.stdout.splitlines()[2]
    assert release.requests[0] == "/pm-v0.2.0-rc.1/SHA256SUMS" and calls(fakes) == []


@pytest.mark.parametrize("broken", ["mismatch", "missing", "unlisted", "not-one-pm"])
def test_a_go_release_that_cannot_be_fetched_or_checked_fails_hard_and_keeps_nothing(fakes, release, broken):
    """A tarball that does not match its SHA256SUMS line, a tarball the release lacks (404), a SHA256SUMS with no
    line for it, a tarball not holding just `pm`: each fails hard naming the release and the URL; nothing is kept."""
    write_config(fakes.root, version="0.2.0")
    tar, sha = publish(release, "0.2.0")
    sums = tar.with_name("SHA256SUMS")
    base, url = f"{release.url}/pm-v0.2.0", f"{release.url}/pm-v0.2.0/{tar.name}"
    head = "error: this repo pins pm 0.2.0, but release pm-v0.2.0"
    if broken == "mismatch":
        sums.write_text(f"{'1' * 64}  {tar.name}\n")
        want = f"{head} could not be checked: {url} has sha256 {sha}, but SHA256SUMS says {'1' * 64}{FIX}\n"
    elif broken == "missing":
        tar.unlink()
        want = f"{head} could not be downloaded: {url}: HTTP 404{FIX}\n"
    elif broken == "unlisted":
        sums.write_text(f"{sha}  pm-0.2.0-plan9-amd64.tar.gz\n")
        want = f"{head} could not be checked: {base}/SHA256SUMS has no line for {tar.name}{FIX}\n"
    else:
        sha = tarball(tar, {"pm": b"#!/bin/sh\n", "extra": b""})
        sums.write_text(f"{sha}  {tar.name}\n")
        want = f"{head} could not be unpacked: {url} is not a gzip tar holding one file pm{FIX}\n"
    res = fakes.pm("show")
    assert (res.returncode, res.stdout, res.stderr) == (1, "", want)
    assert not go_pin(fakes, "0.2.0").exists() and calls(fakes) == []


def test_a_go_release_that_differs_from_the_kept_sha256_fails_hard(fakes, release):
    """The binary is gone but its sha256 is kept: a release is never rebuilt, so a download that differs fails."""
    write_config(fakes.root, version="0.2.0")
    tar, sha = publish(release, "0.2.0")
    kept = go_pin(fakes, "0.2.0") / "sha256"
    kept.parent.mkdir(parents=True)
    kept.write_text("2" * 64 + "\n")
    res = fakes.pm("show")
    assert (res.returncode, res.stdout) == (1, "")
    assert res.stderr == (
        f"error: this repo pins pm 0.2.0, but release pm-v0.2.0 changed since this machine first downloaded it: "
        f"{release.url}/pm-v0.2.0/{tar.name} has sha256 {sha}, but {kept} keeps {'2' * 64}; a release is never "
        "rebuilt, so check where it came from before you delete that file\n")
    assert [p.name for p in kept.parent.iterdir()] == ["sha256"] and kept.read_text() == "2" * 64 + "\n"


@pytest.fixture
def github(fakes, release):
    """The releases of `release` behind the GitHub API stand-in, with no mirror and no token set."""
    gh = GitHub(release.root)
    fakes.env = {k: v for k, v in fakes.env.items() if k not in ("PM_RELEASE_URL", "GH_TOKEN")}
    fakes.env["PM_RELEASE_API"] = gh.api + "/"
    yield gh
    gh.close()


@pytest.mark.parametrize("source", ["GH_TOKEN", "gh auth token"])
def test_a_go_pin_downloads_through_the_github_api_with_a_token_sent_to_the_api_alone(fakes, github, source):
    """With no mirror, the release's assets are found and fetched through the API with the token, from $GH_TOKEN or
    else from gh auth token; the storage host each asset redirects to gets no token."""
    write_config(fakes.root, version="0.2.0")
    tar, sha = publish(SimpleNamespace(root=github.root), "0.2.0")
    fakes.env.update({"GH_TOKEN": TOKEN} if source == "GH_TOKEN" else {"FAKE_GH_TOKEN": TOKEN})
    res = fakes.pm("show")
    assert res.returncode == 0, res.stderr
    assert res.stdout.startswith(f"argv0={go_pin(fakes, '0.2.0') / 'pm'}\narg=show\n")
    assert github.requests == [("/api/releases/tags/pm-v0.2.0", True),
                               ("/api/releases/assets/0", True), ("/files/pm-v0.2.0/SHA256SUMS", False),
                               ("/api/releases/assets/1", True), (f"/files/pm-v0.2.0/{tar.name}", False)]
    assert (go_pin(fakes, "0.2.0") / "sha256").read_text() == sha + "\n" and calls(fakes) == []


def test_a_go_pin_with_no_token_fails_hard_naming_gh_token_and_gh_auth_token(fakes, github):
    write_config(fakes.root, version="0.2.0")
    publish(SimpleNamespace(root=github.root), "0.2.0")
    res = fakes.pm("show")
    assert (res.returncode, res.stdout) == (1, "")
    assert res.stderr == ("error: this repo pins pm 0.2.0, but release pm-v0.2.0 is downloaded through the GitHub API, "
                          "which needs a token: set GH_TOKEN, or log in with gh auth login so that gh auth token "
                          "prints one\n")
    assert github.requests == [] and not go_pin(fakes, "0.2.0").exists()


@pytest.mark.parametrize("broken", ["no-release", "no-asset", "bad-token"])
def test_a_go_release_the_api_cannot_give_fails_hard_and_keeps_nothing(fakes, github, broken):
    write_config(fakes.root, version="0.2.0")
    tar, _ = publish(SimpleNamespace(root=github.root), "0.2.0")
    fakes.env["GH_TOKEN"] = "wrong" if broken == "bad-token" else TOKEN
    url = f"{github.api}/releases/tags/pm-v0.2.0"
    head = f"error: this repo pins pm 0.2.0, but release pm-v0.2.0 could not be downloaded: {url}"
    if broken == "no-release":
        shutil.rmtree(github.root / "pm-v0.2.0")
        want = f"{head}: HTTP 404{FIX}\n"
    elif broken == "no-asset":
        tar.unlink()
        want = f"{head} has no asset {tar.name}{FIX}\n"
    else:
        want = f"{head}: HTTP 401{FIX}\n"
    res = fakes.pm("show")
    assert (res.returncode, res.stdout, res.stderr) == (1, "", want)
    assert not go_pin(fakes, "0.2.0").exists() and calls(fakes) == []


@pytest.mark.integration  # pm upgrade is integration_only's, though this one reaches no remote
def test_upgrade_to_a_go_version_launches_its_release_binary(fakes, release):
    publish(release, "0.2.0")
    res = fakes.pm("upgrade", "--to", "0.2.0")
    assert res.returncode == 0, res.stderr
    assert res.stdout.splitlines()[1:4] == ["arg=upgrade", "arg=--to", "arg=0.2.0"]
    assert (go_pin(fakes, "0.2.0") / "pm").exists() and calls(fakes) == []


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
