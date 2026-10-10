"""The launcher (internal/launch): the installed pm runs each repo's pinned version; a pin below 0.2.0, a retired
Python release, fails hard. A fake `uv` and a fake `git ls-remote` first on PATH log any call, which no launch makes.
A pin runs its release binary: `release` serves releases on 127.0.0.1 through PM_RELEASE_URL, each tarball holding a fake pm script that prints its argv, markers and stdin; `github` serves them as GitHub's API does, for the
download with a token when the one with none fails."""

from __future__ import annotations

import gzip
import hashlib
import http.server
import io
import json
import os
import platform
import shutil
import sys
import tarfile
import threading
from pathlib import Path
from types import SimpleNamespace

import pytest

from conftest import VERSION, write_config

REPO = "https://github.com/Yeeef/pm"

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


RETIRED = (f"this repo pins pm 0.1.5, a Python pm release, retired: pm runs only releases from 0.2.0 on; move the pin "
           f"to a release from 0.2.0 on with pm upgrade --to <X> (releases: {REPO}/releases), and commit "
           ".pm/config.toml")


def test_a_python_pin_fails_hard_naming_the_fix(fakes):
    """A pin below 0.2.0 names a retired Python release: every command refuses, naming how to move the pin, and the
    launcher fetches and runs nothing."""
    write_config(fakes.root, version="0.1.5")
    for args in (["show"], ["where"], ["hook", "stop"]):
        res = fakes.pm(*args, stdin="{}")
        assert (res.returncode, res.stdout, res.stderr) == (1, "", f"error: {RETIRED}\n"), args
    assert calls(fakes) == [] and not (Path(fakes.env["XDG_DATA_HOME"]) / "pm/pins/0.1.5").exists()


@pytest.mark.integration  # pm upgrade is integration_only's, though this one reaches no remote
def test_upgrade_moves_a_python_pin_and_refuses_to_move_one_to_python(fakes):
    """A bare pm upgrade moves a retired pin to the running pm, the fix the refusal names; --to a Python release is
    refused."""
    path = write_config(fakes.root, version="0.1.5")
    res = fakes.pm("upgrade", "--to", "0.1.4")
    assert (res.returncode, res.stderr) == (1, "error: pm upgrade --to 0.1.4 names a Python pm release, retired: pm "
                                               "runs only releases from 0.2.0 on; give a release from 0.2.0 on "
                                               f"(releases: {REPO}/releases)\n"), res.stderr
    res = fakes.pm("upgrade")
    assert res.returncode == 0 and f"moved the pin from 0.1.5 to {VERSION}" in res.stdout, res.stderr
    assert f'version = "{VERSION}"' in path.read_text() and calls(fakes) == []


def test_a_repo_pinned_to_this_version_runs_in_process_with_no_uv_call(fakes):
    res = fakes.pm("where")
    assert res.returncode == 0, res.stderr
    assert res.stdout.splitlines()[0] == f"pm        {VERSION}  run in process: it is this repo's pin, or the repo pins none yet"
    assert calls(fakes) == []


def test_a_pm_launched_for_its_pin_that_runs_another_version_fails_instead_of_launching_again(fakes):
    path = write_config(fakes.root, version="9.0.0")
    fakes.env = dict(fakes.env, PM_LAUNCHED="9.0.0")
    res = fakes.pm("show")
    assert res.returncode == 1 and calls(fakes) == []
    assert res.stderr == (f"error: this repo pins pm 9.0.0 in {path.resolve()}, but pm {VERSION} is running, "
                          f"launched for that pin: release tag pm-v9.0.0 at {REPO} builds pm {VERSION}; "
                          f"fix the tag, or move the pin to {VERSION} with pm upgrade --to {VERSION}\n")


@pytest.mark.integration  # pm upgrade is integration_only's, though this one reaches no remote
def test_upgrade_without_to_never_moves_a_newer_pin_down(fakes):
    """A bare pm upgrade runs at the installed pm's version; in a repo pinned newer it refuses, naming what works."""
    path = write_config(fakes.root, version="9.0.0")
    before = path.read_text()
    res = fakes.pm("upgrade")
    assert res.returncode == 1 and res.stderr == (
        f"error: this repo pins pm 9.0.0, newer than the running pm {VERSION}, and pm upgrade moves a pin down only "
        "when --to names the version; run pm upgrade --to 9.0.0 to rewrite pm's pieces at the pin, or install the "
        f"latest pm with curl -fsSL {REPO}/releases/latest/download/install.sh | sh, then pm upgrade\n"), res.stderr
    assert path.read_text() == before and calls(fakes) == []


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
    token. The API answers 401 without `Bearer TOKEN`. `download` (<download>/<tag>/<asset>) answers 404, as GitHub
    does to a request with no token for a private repo's asset. `requests` logs (path, whether it carried the
    token)."""

    def __init__(self, root: Path):
        self.root, self.requests, self.assets = root, [], []
        gh = self

        class Handler(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                auth = self.headers.get("Authorization")
                gh.requests.append((self.path, auth is not None))
                parts = self.path.strip("/").split("/")
                if parts[0] == "download":  # the assets with no token, of a private repo
                    return self.answer(404, b"Not Found")
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
        self.download = f"http://127.0.0.1:{self.port}/download"
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
                          f"launched=0.2.0 launcher={VERSION}\nstdin=the input\n")
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
    """The releases of `release` behind the GitHub API stand-in, its download with no token failing, and no token
    set."""
    gh = GitHub(release.root)
    fakes.env = {k: v for k, v in fakes.env.items() if k != "GH_TOKEN"}
    fakes.env.update(PM_RELEASE_URL=gh.download, PM_RELEASE_API=gh.api + "/")
    yield gh
    gh.close()


@pytest.mark.parametrize("source", ["GH_TOKEN", "gh auth token"])
def test_a_go_pin_downloads_through_the_github_api_with_a_token_sent_to_the_api_alone(fakes, github, source):
    """When the download with no token fails, the release's assets are found and fetched through the API with the
    token, from $GH_TOKEN or else from gh auth token; the storage host each asset redirects to gets no token."""
    write_config(fakes.root, version="0.2.0")
    tar, sha = publish(SimpleNamespace(root=github.root), "0.2.0")
    fakes.env.update({"GH_TOKEN": TOKEN} if source == "GH_TOKEN" else {"FAKE_GH_TOKEN": TOKEN})
    res = fakes.pm("show")
    assert res.returncode == 0, res.stderr
    assert res.stdout.startswith(f"argv0={go_pin(fakes, '0.2.0') / 'pm'}\narg=show\n")
    assert github.requests == [("/download/pm-v0.2.0/SHA256SUMS", False), ("/api/releases/tags/pm-v0.2.0", True),
                               ("/api/releases/assets/0", True), ("/files/pm-v0.2.0/SHA256SUMS", False),
                               ("/api/releases/assets/1", True), (f"/files/pm-v0.2.0/{tar.name}", False)]
    assert (go_pin(fakes, "0.2.0") / "sha256").read_text() == sha + "\n" and calls(fakes) == []


def test_a_go_pin_that_fails_with_no_token_and_has_none_fails_hard_naming_that_download(fakes, github):
    write_config(fakes.root, version="0.2.0")
    publish(SimpleNamespace(root=github.root), "0.2.0")
    res = fakes.pm("show")
    assert (res.returncode, res.stdout) == (1, "")
    assert res.stderr == (f"error: this repo pins pm 0.2.0, but release pm-v0.2.0 could not be downloaded: "
                          f"{github.download}/pm-v0.2.0/SHA256SUMS: HTTP 404{FIX}\n")
    assert github.requests == [("/download/pm-v0.2.0/SHA256SUMS", False)] and not go_pin(fakes, "0.2.0").exists()


def test_a_go_pin_downloaded_with_no_token_never_asks_for_one(fakes, release):
    """A token at hand is not used when the download with none works."""
    write_config(fakes.root, version="0.2.0")
    tar, _ = publish(release, "0.2.0")
    fakes.env.update(GH_TOKEN=TOKEN, PM_RELEASE_API="http://127.0.0.1:1/api")  # never asked
    res = fakes.pm("show")
    assert res.returncode == 0, res.stderr
    assert release.requests == ["/pm-v0.2.0/SHA256SUMS", f"/pm-v0.2.0/{tar.name}"]


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
