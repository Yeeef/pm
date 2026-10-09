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

from test_launch import PLATFORM, tarball

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
