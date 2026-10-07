"""The launcher: the pm uv tool runs each repo's pinned pm version, so repos on different pins share one machine.

main() calls launch() before anything else. It reads only `version` in the repo's `.pm/config.toml` (config.root
picks the checkout, as every command does) and runs the command in this process when there is no readable pin, when
the pin is this pm's version, for `pm upgrade` (it moves the pin to the running pm; `--to X` launches pm X instead),
and when this process was launched for that pin already ($PM_LAUNCHED): a release that builds another version then
fails the config check instead of launching again. Otherwise it replaces this process with
`uv tool run --from git+<REPO>@<commit>#subdirectory=pm pm <args>`, so stdin, stdout, stderr, the pid and the exit
code are the launched pm's.

A tag makes uv fetch on every run (6 s measured), a commit runs from uv's cache (0.2 s). So the first launch of a pin
on a machine resolves its tag to a commit (git ls-remote), runs it once to fetch and build it, and only then keeps the
commit in `<data dir>/pm/pins/<pin>/commit`; a failure there fails hard naming the tag and the command.

A pin older than 0.1.2 predates the launcher: its pm init, also run at session start, reinstalls the pm uv tool at
its own version. It runs with UV_TOOL_DIR and UV_TOOL_BIN_DIR in its pin's directory, that bin dir first on PATH, so
its tool and the service unit it writes stay there and the machine's pm uv tool stays the launcher."""

from __future__ import annotations

import os
import subprocess
import sys
import tomllib
from pathlib import Path

from pm import __version__, config

LAUNCHED = "PM_LAUNCHED"  # the pin this process was launched for
LAUNCHER = "PM_LAUNCHER"  # the version of the pm that launched it
FIRST = (0, 1, 2)  # the first pm that knows it was launched


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


def old(version: str) -> bool:
    """Whether `version` predates the launcher."""
    try:
        return tuple(int(p) for p in version.split(".")) < FIRST
    except ValueError:
        return False


def pin_dir(version: str) -> Path:
    return Path(os.environ.get("XDG_DATA_HOME") or Path.home() / ".local/share") / "pm/pins" / version


def requirement(commit: str) -> str:
    return f"git+{config.REPO}@{commit}#subdirectory=pm"


def environment(version: str) -> dict:
    """The launched pm's environment: the markers, and for a pin older than the launcher its own uv tool dirs."""
    env = dict(os.environ, **{LAUNCHED: version, LAUNCHER: __version__})
    if old(version):
        d = pin_dir(version)
        env.update(UV_TOOL_DIR=str(d / "tools"), UV_TOOL_BIN_DIR=str(d / "bin"),
                   PATH=f"{d / 'bin'}{os.pathsep}{env.get('PATH', '')}")
    return env


def commit(version: str, env: dict) -> str:
    """The commit release tag pm-v<version> names, fetched and built once; kept after the first launch."""
    kept = pin_dir(version) / "commit"
    if kept.is_file():
        return kept.read_text().strip()
    tag = f"pm-v{version}"
    fix = (f"check the network and that tag {tag} exists, then run pm again; or run it yourself with uv tool run "
           f"--from \"{config.RELEASE.format(v=version)}\" pm …, or move the pin with pm upgrade")
    head = f"this repo pins pm {version}, which the pm uv tool (pm {__version__}) runs through uv"
    try:
        res = subprocess.run(["git", "ls-remote", config.REPO, f"refs/tags/{tag}", f"refs/tags/{tag}^{{}}"],
                             capture_output=True, text=True, stdin=subprocess.DEVNULL)
    except FileNotFoundError:
        raise LaunchError(f"{head}, but git is not installed to find release tag {tag}")
    refs = {ref: sha for sha, ref in (line.split("\t", 1) for line in res.stdout.splitlines() if "\t" in line)}
    sha = refs.get(f"refs/tags/{tag}^{{}}") or refs.get(f"refs/tags/{tag}")
    if res.returncode != 0 or not sha:
        why = (res.stderr.strip().splitlines() or ["no such tag"])[-1]
        raise LaunchError(f"{head}, but release tag {tag} was not found at {config.REPO} ({why}); {fix}")
    try:
        res = subprocess.run(["uv", "tool", "run", "--from", requirement(sha), "pm", "--help"], env=env,
                             stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, text=True)
    except FileNotFoundError:
        raise LaunchError(f"{head}, but uv is not installed (https://docs.astral.sh/uv/)")
    if res.returncode != 0:
        why = (res.stderr.strip().splitlines() or [f"exit {res.returncode}"])[-1].strip()
        raise LaunchError(f"{head}, but uv could not fetch and build {tag} ({sha}): {why}; {fix}")
    kept.parent.mkdir(parents=True, exist_ok=True)
    kept.write_text(sha + "\n")
    return sha


def launch(argv: list[str]) -> int | None:
    """Run `pm <argv>` as the pinned version: None to run it in this process, else replace this process (no
    return), or 1 when the pinned version cannot run, said on stderr."""
    version = target(argv, Path.cwd(), os.environ)
    if version is None:
        return None
    env = environment(version)
    try:
        cmd = ["uv", "--quiet", "tool", "run", "--from", requirement(commit(version, env)), "pm", *argv]
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
    if os.environ.get(LAUNCHED) == __version__:
        return f"this repo's pin, launched by the pm uv tool (pm {os.environ.get(LAUNCHER) or 'unknown'})"
    return "run in process: it is this repo's pin, or the repo pins none yet"


def launched() -> bool:
    """Whether this pm was launched for its own version: the pm uv tool on this machine is another version."""
    return os.environ.get(LAUNCHED) == __version__
