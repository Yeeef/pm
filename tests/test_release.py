"""Go pm's release tooling: install.sh against a local release server, and the release build (pm/release/build.sh),
which takes the version from the release tag alone. Neither runs a pm implementation under test, so both run on
Python's suite only."""

from __future__ import annotations

import functools
import http.server
import os
import subprocess
import tarfile
import threading
from pathlib import Path
from types import SimpleNamespace

import pytest

from test_launch import PLATFORM, TOKEN, GitHub, tarball

PM_DIR = Path(__file__).resolve().parents[1]
TOOLING = pytest.mark.impl("python", reason="tests the release tooling, not a pm implementation")
FAKE = b"#!/bin/sh\necho the installed pm\n"


@pytest.fixture
def served(tmp_path):
    """Release pm-v0.2.0 under a local HTTP server: this platform's tarball and SHA256SUMS; install.sh with its
    version filled in, as the release workflow serves it; the env that points install.sh at both and a temp bin dir."""
    root = tmp_path / "releases"
    d = root / "pm-v0.2.0"
    d.mkdir(parents=True)
    tar = d / f"pm-0.2.0-{PLATFORM}.tar.gz"
    sha = tarball(tar, {"pm": FAKE})
    (d / "SHA256SUMS").write_text(f"{'0' * 64}  pm-0.2.0-plan9-amd64.tar.gz\n{sha}  {tar.name}\n")
    script = tmp_path / "install.sh"
    script.write_text((PM_DIR / "install.sh").read_text().replace("@VERSION@", "0.2.0"))
    class Quiet(http.server.SimpleHTTPRequestHandler):
        def log_message(self, *args):
            pass

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), functools.partial(Quiet, directory=str(root)))
    threading.Thread(target=server.serve_forever, daemon=True).start()
    url = f"http://127.0.0.1:{server.server_port}"
    env = dict(os.environ, HOME=str(tmp_path / "home"), PM_BIN_DIR=str(tmp_path / "bin"), PM_RELEASE_URL=url + "/")
    yield SimpleNamespace(tar=tar, sha=sha, script=script, env=env, bin=tmp_path / "bin", url=f"{url}/pm-v0.2.0")
    server.shutdown()
    server.server_close()


@TOOLING
def test_install_sh_installs_the_checked_binary_into_the_bin_dir(served):
    res = subprocess.run(["sh", str(served.script)], env=served.env, capture_output=True, text=True)
    assert res.returncode == 0, res.stderr
    pm = served.bin / "pm"
    assert pm.read_bytes() == FAKE and pm.stat().st_mode & 0o777 == 0o755
    assert [p.name for p in served.bin.iterdir()] == ["pm"], "no temp file is left"
    assert res.stdout.splitlines() == [
        f"installed pm 0.2.0 at {pm} (from {served.url}/{served.tar.name}, sha256 {served.sha})",
        f"{served.bin} is not on PATH: add it, e.g. export PATH=\"{served.bin}:$PATH\" in your shell's profile",
        "next, set up each clone that uses pm: run pm init in it"]
    assert subprocess.run([str(pm)], capture_output=True, text=True).stdout == "the installed pm\n"


@TOOLING
def test_install_sh_refuses_a_checksum_mismatch_and_installs_nothing(served):
    sums = served.tar.with_name("SHA256SUMS")
    sums.write_text(f"{'1' * 64}  {served.tar.name}\n")
    served.bin.mkdir()
    (served.bin / "pm").write_bytes(b"the pm before\n")
    res = subprocess.run(["sh", str(served.script)], env=served.env, capture_output=True, text=True)
    assert (res.returncode, res.stdout) == (1, "")
    assert res.stderr == (f"install.sh: error: {served.url}/{served.tar.name} has sha256 {served.sha}, but SHA256SUMS "
                          f"says {'1' * 64}; nothing was installed\n")
    assert [p.name for p in served.bin.iterdir()] == ["pm"] and (served.bin / "pm").read_bytes() == b"the pm before\n"


@TOOLING
def test_install_sh_refuses_a_release_with_no_line_for_this_platform(served):
    served.tar.with_name("SHA256SUMS").write_text(f"{served.sha}  pm-0.2.0-plan9-amd64.tar.gz\n")
    res = subprocess.run(["sh", str(served.script)], env=served.env, capture_output=True, text=True)
    assert res.returncode == 1
    assert res.stderr == f"install.sh: error: {served.url}/SHA256SUMS has no line for {served.tar.name}\n"
    assert not served.bin.exists()


@pytest.fixture
def api(served, tmp_path):
    """`served`'s release behind the GitHub API stand-in, with no mirror and no token; a gh first on PATH whose
    `gh auth token` prints $FAKE_GH_TOKEN, and without it fails as gh does when not logged in."""
    gh = GitHub(served.tar.parent.parent)
    bindir = tmp_path / "fakebin"
    bindir.mkdir()
    (bindir / "gh").write_text('#!/bin/sh\n[ "$*" = "auth token" ] && [ -n "${FAKE_GH_TOKEN:-}" ] || exit 1\n'
                               'echo "$FAKE_GH_TOKEN"\n')
    (bindir / "gh").chmod(0o755)
    env = {k: v for k, v in served.env.items() if k not in ("PM_RELEASE_URL", "GH_TOKEN")}
    env.update(PM_RELEASE_API=gh.api + "/", PATH=f"{bindir}{os.pathsep}{env['PATH']}")
    yield SimpleNamespace(gh=gh, env=env)
    gh.close()


@TOOLING
@pytest.mark.parametrize("source", ["GH_TOKEN", "gh auth token"])
def test_install_sh_downloads_through_the_github_api_with_a_token_sent_to_the_api_alone(served, api, source):
    env = dict(api.env, **({"GH_TOKEN": TOKEN} if source == "GH_TOKEN" else {"FAKE_GH_TOKEN": TOKEN}))
    res = subprocess.run(["sh", str(served.script)], env=env, capture_output=True, text=True)
    assert res.returncode == 0, res.stderr
    assert (served.bin / "pm").read_bytes() == FAKE
    tar_url = f"{api.gh.api}/releases/assets/{api.gh.assets.index(('pm-v0.2.0', served.tar.name))}"
    assert res.stdout.splitlines()[0] == f"installed pm 0.2.0 at {served.bin / 'pm'} (from {tar_url}, sha256 {served.sha})"
    assert [r for r in api.gh.requests if r[0].startswith("/files/")] == [
        ("/files/pm-v0.2.0/SHA256SUMS", False), (f"/files/pm-v0.2.0/{served.tar.name}", False)]
    assert all(token for path, token in api.gh.requests if path.startswith("/api/"))


@TOOLING
def test_install_sh_with_no_token_fails_hard_naming_gh_token_and_gh_auth_token(served, api):
    res = subprocess.run(["sh", str(served.script)], env=api.env, capture_output=True, text=True)
    assert (res.returncode, res.stdout) == (1, "")
    assert res.stderr == ("install.sh: error: release pm-v0.2.0 is downloaded through the GitHub API, which needs a "
                          "token: set GH_TOKEN, or log in with gh auth login so that gh auth token prints one\n")
    assert api.gh.requests == [] and not served.bin.exists()


@TOOLING
def test_install_sh_refuses_a_release_without_this_platforms_asset(served, api):
    served.tar.unlink()
    res = subprocess.run(["sh", str(served.script)], env=dict(api.env, GH_TOKEN=TOKEN), capture_output=True, text=True)
    assert res.returncode == 1
    assert res.stderr == (f"install.sh: error: {api.gh.api}/releases/tags/pm-v0.2.0 has no asset {served.tar.name}\n")
    assert not served.bin.exists()


def version_of(tar: Path, tmp: Path) -> str:
    """`pm version` of the binary the release tarball holds, run outside any repo."""
    with tarfile.open(tar) as tf:
        assert [(m.name, m.isreg(), m.mode) for m in tf.getmembers()] == [("pm", True, 0o755)]
        tf.extractall(tmp, filter="tar")
    res = subprocess.run([str(tmp / "pm"), "version"], cwd=tmp, capture_output=True, text=True)
    assert res.returncode == 0, res.stderr
    return res.stdout


@TOOLING
@pytest.mark.integration
@pytest.mark.skipif(not os.environ.get("PM_RELEASE_BUILD"), reason="builds Go pm twice; pm-go.yml sets PM_RELEASE_BUILD")
def test_the_release_build_takes_its_version_from_the_tag_alone(tmp_path):
    """A scratch clone at this commit, nothing committed: tagged pm-v<X>, the release build reports X; untagged, dev."""
    clone = tmp_path / "clone"
    subprocess.run(["git", "clone", "-q", str(PM_DIR.parent), str(clone)], check=True)
    subprocess.run(["git", "-C", str(clone), "checkout", "-q", "--detach",
                    subprocess.run(["git", "-C", str(PM_DIR), "rev-parse", "HEAD"], capture_output=True, text=True,
                                   check=True).stdout.strip()], check=True)
    for tag in subprocess.run(["git", "-C", str(clone), "tag", "-l", "pm-v*"], capture_output=True, text=True,
                              check=True).stdout.split():  # a release tag already on this commit would name it
        subprocess.run(["git", "-C", str(clone), "tag", "-d", tag], check=True, capture_output=True)
    build = [str(clone / "pm/release/build.sh"), str(tmp_path / "out")]
    goos, goarch = (subprocess.run(["go", "env", v], cwd=PM_DIR, capture_output=True, text=True, check=True)
                    .stdout.strip() for v in ("GOOS", "GOARCH"))

    subprocess.run(["git", "-C", str(clone), "tag", "pm-v0.2.0-rc.42"], check=True)
    res = subprocess.run(build, capture_output=True, text=True)
    assert res.returncode == 0, res.stderr
    tar = tmp_path / f"out/pm-0.2.0-rc.42-{goos}-{goarch}.tar.gz"
    assert res.stdout == f"{tar}\n"
    (tmp_path / "tagged").mkdir()
    assert version_of(tar, tmp_path / "tagged") == "0.2.0-rc.42\n"

    subprocess.run(["git", "-C", str(clone), "tag", "-d", "pm-v0.2.0-rc.42"], check=True, capture_output=True)
    res = subprocess.run(build, capture_output=True, text=True)
    assert res.returncode == 0, res.stderr
    (tmp_path / "untagged").mkdir()
    assert version_of(tmp_path / f"out/pm-dev-{goos}-{goarch}.tar.gz", tmp_path / "untagged") == "dev\n"
    assert subprocess.run(["git", "-C", str(clone), "status", "--porcelain"], capture_output=True, text=True,
                          check=True).stdout == "", "the build writes nothing in the clone"
