"""The launcher: the pm uv tool runs each repo's pinned pm version, so repos on different pins share one machine.

main() calls launch() before anything else. It reads only `version` in the repo's `.pm/config.toml` (config.root
picks the checkout, as every command does) and runs the command in this process when there is no readable pin, when
the pin is this pm's version, for `pm upgrade` (it moves the pin to the running pm; `--to X` launches pm X instead),
and when this process was launched for that pin already ($PM_LAUNCHED): a release that builds another version then
fails the config check instead of launching again. Otherwise it replaces this process with
`uv tool run --from git+<REPO>@<commit>#subdirectory=pm pm <args>`, so stdin, stdout, stderr, the pid and the exit
code are the launched pm's. The markers are for that launched pm alone: launch() takes them out of os.environ into
`marks`, so its children (git hooks, `pm push`'s `claude -p`, any `pm` it runs) reach the launcher afresh.

A tag makes uv fetch on every run (6 s measured), a commit runs from uv's cache (0.2 s). So the first launch of a pin
on a machine resolves its tag to a commit (git ls-remote), runs it once to fetch and build it, and only then keeps the
commit in `<data dir>/pm/pins/<pin>/commit`; a failure or timeout there fails hard naming the tag and the command.
Once kept, uv runs the commit from its cache, or fetches it again after `uv cache clean`, which needs the network;
deleting the commit file makes the next launch resolve the tag again.

A pin older than 0.1.2 predates the launcher: its pm init, also run at session start, reinstalls the pm uv tool at
its own version. It runs with UV_TOOL_DIR and UV_TOOL_BIN_DIR in its pin's directory, that bin dir first on PATH, so
its tool and the service unit it writes stay there and the machine's pm uv tool stays the launcher. It gets no
markers: it never reads them, its children would inherit them, and its release tags build no launcher, so it cannot
launch again. launch() drops those dirs from a child's environment before it picks the pin.

A pin at or above GO (0.2.0, and pre-releases such as 0.2.0-rc.1) is Go pm: a release binary, not a uv build. Its
launch execs `<data dir>/pm/pins/<pin>/pm` with the same markers, with no network once the binary is there. The first
launch downloads it from release pm-v<pin> ($PM_RELEASE_URL, else GitHub's release downloads): SHA256SUMS and this
platform's tarball, pm-<pin>-<os>-<arch>.tar.gz (darwin-arm64 and linux-amd64 only), within 10 s to connect and 300 s
in all. The tarball must match its SHA256SUMS line and the sha256 kept in `pins/<pin>/sha256` by an earlier download,
if any: a release is never rebuilt, so a difference fails hard. The binary, then its sha256, is written to a temp
file and renamed into place, so another launch sees no file or the whole one. Every failure is a hard error naming
the release and the URL; nothing falls back to another version or to uv. `pm upgrade --to <Go version>` launches
that version the same way."""

from __future__ import annotations

import gzip
import hashlib
import io
import os
import platform
import re
import subprocess
import sys
import tarfile
import tempfile
import time
import tomllib
import urllib.error
import urllib.request
import zlib
from pathlib import Path

from pm import __version__, config

LAUNCHED = "PM_LAUNCHED"  # the pin this process was launched for
LAUNCHER = "PM_LAUNCHER"  # the version of the pm that launched it
FIRST = (0, 1, 2)  # the first pm that knows it was launched
RESOLVE_TIMEOUT = 10  # seconds git ls-remote may take to resolve a release tag
BUILD_TIMEOUT = 300  # seconds uv may take to fetch and build a release the first time
GO = (0, 2, 0)  # the first Go pm: a pin at or above it runs its release binary
RELEASES = "https://github.com/Yeeef/yeeef-agents/releases/download"  # $PM_RELEASE_URL overrides it
PLATFORMS = ("darwin-arm64", "linux-amd64")  # the platforms a Go release has a binary for
ARCH = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64", "amd64": "amd64"}
CONNECT_TIMEOUT = 10  # seconds a release download may take to connect, and to answer each read
DOWNLOAD_TIMEOUT = 300  # seconds a release's SHA256SUMS and tarball may take in all
marks: dict[str, str] = {}  # LAUNCHED and LAUNCHER as this process got them; launch() takes them out of os.environ


class LaunchError(Exception):
    """The pinned version cannot run: uv is missing, or its release cannot be found or built."""


def pin(cwd: Path) -> str | None:
    """The version the config of the checkout containing `cwd` pins; None when there is no readable one, for the
    config check to report."""
    try:
        data = tomllib.loads((config.root(cwd) / config.REL).read_text())
    except (config.ConfigError, OSError, tomllib.TOMLDecodeError):
        return None
    return data.get("version") if isinstance(data.get("version"), str) else None


def target(argv: list[str], cwd: Path, env: dict) -> str | None:
    """The version to launch `pm <argv>` into; None to run it in this process."""
    if argv[:1] == ["upgrade"]:
        want = next((a.removeprefix("--to=") for a in argv if a.startswith("--to=")), None)
        if "--to" in argv[:-1]:
            want = argv[argv.index("--to") + 1]
    else:
        want = pin(cwd)
    return None if want in (None, __version__, env.get(LAUNCHED)) else want


def key(version: str) -> tuple[int, ...] | None:
    """`version` as numbers to compare, a pre-release's "-…" suffix left out; None when it is not dotted numbers."""
    try:
        return tuple(int(p) for p in version.split("-", 1)[0].split("."))
    except ValueError:
        return None


def old(version: str) -> bool:
    """Whether `version` predates the launcher."""
    k = key(version)
    return k is not None and k < FIRST


def go(version: str) -> bool:
    """Whether `version` is Go pm, run from its release binary."""
    k = key(version)
    return k is not None and k >= GO


def pins() -> Path:
    return Path(os.environ.get("XDG_DATA_HOME") or Path.home() / ".local/share") / "pm/pins"


def pin_dir(version: str) -> Path:
    return pins() / version


def kept(version: str) -> str | None:
    """The commit kept for `version`; None when there is none or the file holds no commit sha."""
    try:
        sha = (pin_dir(version) / "commit").read_text().strip()
    except OSError:
        return None
    return sha if re.fullmatch(r"[0-9a-f]{40}", sha) else None


def scrub() -> None:
    """Take the markers out of os.environ into `marks`, and an old pin's uv tool dirs, which a pm run by an old pin
    inherits, out of UV_TOOL_DIR, UV_TOOL_BIN_DIR and PATH."""
    marks.clear()
    marks.update({k: os.environ.pop(k) for k in (LAUNCHED, LAUNCHER) if k in os.environ})
    root = pins()
    inside = lambda p: bool(p) and Path(p).is_relative_to(root)
    for k in ("UV_TOOL_DIR", "UV_TOOL_BIN_DIR"):
        if inside(os.environ.get(k)):
            del os.environ[k]
    if "PATH" in os.environ:
        os.environ["PATH"] = os.pathsep.join(p for p in os.environ["PATH"].split(os.pathsep) if not inside(p))


def requirement(commit: str) -> str:
    return f"git+{config.REPO}@{commit}#subdirectory=pm"


def environment(version: str) -> dict:
    """The launched pm's environment: the markers, or for a pin older than the launcher its own uv tool dirs."""
    env = dict(os.environ)
    if not old(version):
        return dict(env, **{LAUNCHED: version, LAUNCHER: __version__})
    d = pin_dir(version)
    env.update(UV_TOOL_DIR=str(d / "tools"), UV_TOOL_BIN_DIR=str(d / "bin"),
               PATH=f"{d / 'bin'}{os.pathsep}{env.get('PATH', '')}")
    return env


def commit(version: str, env: dict) -> str:
    """The commit release tag pm-v<version> names, fetched and built once; kept after the first launch."""
    sha = kept(version)
    if sha:
        return sha
    tag = f"pm-v{version}"
    fix = (f"check the network and that tag {tag} exists, then run pm again; or run it yourself with uv tool run "
           f"--from \"{config.RELEASE.format(v=version)}\" pm …, or move the pin with pm upgrade")
    head = f"this repo pins pm {version}, which the pm uv tool (pm {__version__}) runs through uv"
    try:
        res = subprocess.run(["git", "ls-remote", config.REPO, f"refs/tags/{tag}", f"refs/tags/{tag}^{{}}"],
                             capture_output=True, text=True, stdin=subprocess.DEVNULL, timeout=RESOLVE_TIMEOUT,
                             env=dict(os.environ, GIT_TERMINAL_PROMPT="0"))
    except FileNotFoundError:
        raise LaunchError(f"{head}, but git is not installed to find release tag {tag}")
    except subprocess.TimeoutExpired:
        raise LaunchError(f"{head}, but git ls-remote {config.REPO} did not answer within {RESOLVE_TIMEOUT} s for "
                          f"release tag {tag}; {fix}")
    refs = {ref: sha for sha, ref in (line.split("\t", 1) for line in res.stdout.splitlines() if "\t" in line)}
    sha = refs.get(f"refs/tags/{tag}^{{}}") or refs.get(f"refs/tags/{tag}")
    if res.returncode != 0 or not sha:
        why = (res.stderr.strip().splitlines() or ["no such tag"])[-1]
        raise LaunchError(f"{head}, but release tag {tag} was not found at {config.REPO} ({why}); {fix}")
    try:
        res = subprocess.run(["uv", "tool", "run", "--from", requirement(sha), "pm", "--help"], env=env,
                             stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, text=True,
                             timeout=BUILD_TIMEOUT)
    except FileNotFoundError:
        raise LaunchError(f"{head}, but uv is not installed (https://docs.astral.sh/uv/)")
    except subprocess.TimeoutExpired:
        raise LaunchError(f"{head}, but uv did not fetch and build {tag} ({sha}) within {BUILD_TIMEOUT} s; {fix}")
    if res.returncode != 0:
        why = (res.stderr.strip().splitlines() or [f"exit {res.returncode}"])[-1].strip()
        raise LaunchError(f"{head}, but uv could not fetch and build {tag} ({sha}): {why}; {fix}")
    path = pin_dir(version) / "commit"
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_name(f"commit.{os.getpid()}")  # another launch may read it at once: it sees no file or the whole sha
    tmp.write_text(sha + "\n")
    os.replace(tmp, path)
    return sha


def binary(version: str) -> Path:
    """Go pm `version`'s binary: the kept one, or downloaded from its release, checked and kept."""
    d = pin_dir(version)
    path, sha_file = d / "pm", d / "sha256"
    if path.is_file() and os.access(path, os.X_OK):
        return path
    tag = f"pm-v{version}"
    base = f"{(os.environ.get('PM_RELEASE_URL') or RELEASES).rstrip('/')}/{tag}"
    head = f"this repo pins pm {version}, but release {tag}"
    fix = "; check the network and the release, then run pm again, or move the pin with pm upgrade"
    plat = this_platform()
    if plat not in PLATFORMS:
        raise LaunchError(f"{head} has no binary for {plat} (only {' and '.join(PLATFORMS)}): {base}")
    asset = f"pm-{version}-{plat}.tar.gz"
    sums_url, tar_url = f"{base}/SHA256SUMS", f"{base}/{asset}"
    deadline = time.monotonic() + DOWNLOAD_TIMEOUT
    sums = io.BytesIO()
    download(sums_url, sums, deadline, f"{head} could not be downloaded: {sums_url}: ", fix)
    lines = [line.split() for line in sums.getvalue().decode(errors="replace").splitlines()]
    want = next((f[0] for f in lines if len(f) == 2 and f[1] == asset and re.fullmatch(r"[0-9a-f]{64}", f[0])), None)
    if want is None:
        raise LaunchError(f"{head} could not be checked: {sums_url} has no line for {asset}{fix}")
    with tempfile.TemporaryFile() as tar:
        got = download(tar_url, tar, deadline, f"{head} could not be downloaded: {tar_url}: ", fix)
        if got != want:
            raise LaunchError(f"{head} could not be checked: {tar_url} has sha256 {got}, but SHA256SUMS says "
                              f"{want}{fix}")
        before = sha_file.read_text().strip() if sha_file.exists() else None
        if before is not None and before != got:
            raise LaunchError(f"{head} changed since this machine first downloaded it: {tar_url} has sha256 {got}, "
                              f"but {sha_file} keeps {before}; a release is never rebuilt, so check where it came "
                              "from before you delete that file")
        tar.seek(0)
        bad = f"{head} could not be unpacked: {tar_url} is not a gzip tar holding one file pm{fix}"
        unpacked = (tarfile.TarError, EOFError, zlib.error, gzip.BadGzipFile)
        try:
            tf = tarfile.open(fileobj=tar, mode="r:gz")
            members = tf.getmembers()
        except unpacked as e:
            raise LaunchError(bad) from e
        if [(m.name, m.isreg()) for m in members] != [("pm", True)]:
            raise LaunchError(bad)
        d.mkdir(parents=True, exist_ok=True)
        with tf, tempfile.NamedTemporaryFile(dir=d, prefix="pm.", delete=False) as out:
            tmp = Path(out.name)
            try:
                src = tf.extractfile(members[0])
                while chunk := src.read(1 << 20):
                    out.write(chunk)
            except unpacked as e:
                tmp.unlink()
                raise LaunchError(bad) from e
            except BaseException:
                tmp.unlink()
                raise
    tmp.chmod(0o755)
    os.replace(tmp, path)
    tmp = d / f"sha256.{os.getpid()}"
    tmp.write_text(got + "\n")
    os.replace(tmp, sha_file)
    return path


def this_platform() -> str:
    """The machine's <os>-<arch> as release assets name it. An x86_64 Python under Rosetta (as uv may install on
    Apple silicon) reports x86_64, while the machine runs the arm64 binary natively, so a translated process is arm64."""
    machine = platform.machine().lower()
    if sys.platform == "darwin" and machine == "x86_64":
        res = subprocess.run(["/usr/sbin/sysctl", "-n", "sysctl.proc_translated"], capture_output=True, text=True,
                             stdin=subprocess.DEVNULL)
        machine = "arm64" if res.stdout.strip() == "1" else machine
    return f"{sys.platform}-{ARCH.get(machine, machine)}"


def download(url: str, out, deadline: float, why: str, fix: str) -> str:
    """Write the body of GET `url` to `out` by `deadline`; its sha256 hex. `why` and `fix` frame a failure's error."""
    sha = hashlib.sha256()
    try:
        with urllib.request.urlopen(url, timeout=CONNECT_TIMEOUT) as res:
            while chunk := res.read(1 << 20):
                if time.monotonic() > deadline:
                    raise LaunchError(f"{why}not downloaded within {DOWNLOAD_TIMEOUT} s{fix}")
                sha.update(chunk)
                out.write(chunk)
    except urllib.error.HTTPError as e:
        raise LaunchError(f"{why}HTTP {e.code}{fix}")
    except urllib.error.URLError as e:
        reason = f"no answer within {CONNECT_TIMEOUT} s" if isinstance(e.reason, TimeoutError) else e.reason
        raise LaunchError(f"{why}{reason}{fix}")
    except TimeoutError:
        raise LaunchError(f"{why}no answer within {CONNECT_TIMEOUT} s{fix}")
    except OSError as e:
        raise LaunchError(f"{why}{e}{fix}")
    return sha.hexdigest()


def launch(argv: list[str]) -> int | None:
    """Run `pm <argv>` as the pinned version: None to run it in this process, else replace this process (no
    return), or 1 when the pinned version cannot run, said on stderr."""
    scrub()
    version = target(argv, Path.cwd(), marks)
    if version is None:
        return None
    env = environment(version)
    try:
        if go(version):
            path = binary(version)
            try:
                os.execve(path, [str(path), *argv], env)
            except OSError as e:
                raise LaunchError(f"this repo pins pm {version}, but {path} could not run: {e.strerror}; delete it "
                                  "to download it again")
        cmd =["uv", "--quiet", "tool", "run", "--from", requirement(commit(version, env)), "pm", *argv]
        try:
            os.execvpe("uv", cmd, env)
        except FileNotFoundError:
            raise LaunchError(f"this repo pins pm {version}, which the pm uv tool (pm {__version__}) runs through uv, "
                              "but uv is not installed (https://docs.astral.sh/uv/)")
    except LaunchError as e:
        print(f"error: {e}", file=sys.stderr)
        return 1


def how() -> str:
    """How the running pm was chosen, for pm where and pm doctor."""
    if launched():
        return (f"this repo's pin at commit {kept(__version__) or 'unknown'}, launched by the pm uv tool "
                f"(pm {marks.get(LAUNCHER) or 'unknown'}); delete {pin_dir(__version__) / 'commit'} to resolve "
                f"tag pm-v{__version__} again")
    return "run in process: it is this repo's pin, or the repo pins none yet"


def launched() -> bool:
    """Whether this pm was launched for its own version: the pm uv tool on this machine is another version."""
    return marks.get(LAUNCHED) == __version__
