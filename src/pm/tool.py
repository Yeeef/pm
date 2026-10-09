"""The pm uv tool: the one pm a machine runs, which `pm init` installs and the pm service's unit runs.

`uvx --from "git+…@pm-v<X>" pm init` runs pm from an ephemeral uv cache environment, which
`uv cache clean` removes; the service and the hooks need a pm that stays. So `pm init` installs the tool from the
source the running pm came from (PEP 610's direct_url.json: the git URL and commit), unless the tool already runs
this build. A pm from a local directory or an index has no source to install from, and the tool, installed from
git, never runs its build, so every command that installs or checks the tool or the service refuses to run from
one, naming the command that runs pm as the tool. A build is the version and the commit and subdirectory it was
built from, not the URL as typed, so a tool built from another commit of the same version is replaced, and the
service, which sends its build, is seen as stale, while one commit spelled two ways is one build. The service's
unit runs the tool's interpreter, whose path stays the same across versions.

The tool is also the launcher (launch.py): in a repo pinned to another version it runs that version. A pm so
launched never installs the tool, which stays the machine's launcher, and takes any launcher as current; its own
build is what the service it starts sends, so the stale check compares the pin's build with the pin's build."""

from __future__ import annotations

import importlib.metadata
import json
import os
import shutil
import subprocess
import sys
from pathlib import Path

from pm import __version__
from .config import INSTALL, RELEASE
from .launch import FIRST, launched, old
from .records import RecordError

NAME = "pm"


def uv(*args: str) -> str:
    try:
        res = subprocess.run(["uv", "--color", "never", *args], capture_output=True, text=True)
    except FileNotFoundError:
        raise RecordError("uv is not installed; pm is a uv tool and needs it (https://docs.astral.sh/uv/)")
    if res.returncode != 0:
        raise RecordError(f"uv {' '.join(args)} failed: {(res.stderr or res.stdout).strip()}")
    return res.stdout.strip()


def python() -> Path:
    """The tool environment's interpreter, whether or not the tool is installed."""
    return Path(uv("tool", "dir")) / NAME / "bin" / "python"


# what the tool's interpreter prints: its pm's version and direct_url.json (null when it has none)
PROBE = ("import importlib.metadata, json, pm; "
         "print(json.dumps([pm.__version__, importlib.metadata.distribution('pm').read_text('direct_url.json')]))")


def origin(raw: str | None) -> str | None:
    """The source a direct_url.json names: the requirement that installs that commit again for a git install, the
    URL for a local directory; None when there is none (an install from an index)."""
    info = json.loads(raw) if raw else {}
    vcs = info.get("vcs_info") or {}
    if vcs.get("vcs") == "git" and vcs.get("commit_id"):
        sub = f"#subdirectory={info['subdirectory']}" if info.get("subdirectory") else ""
        return f"git+{info['url']}@{vcs['commit_id']}{sub}"
    return info.get("url")


def build(version: str, raw: str | None) -> str:
    """A pm build, as pm compares it: its version and, for a git install, the commit and subdirectory it was built
    from, not the URL, which one commit may be typed as several ways; else the source it names, if any."""
    info = json.loads(raw) if raw else {}
    vcs = info.get("vcs_info") or {}
    if vcs.get("vcs") == "git" and vcs.get("commit_id"):
        sub = f" in {info['subdirectory']}" if info.get("subdirectory") else ""
        return f"{version} at {vcs['commit_id']}{sub}"
    return f"{version} from {info['url']}" if info.get("url") else version


def shown(version: str, raw: str | None) -> str:
    """A pm build as pm names it: its version and the requirement it was installed from, when it names one."""
    src = origin(raw)
    return f"{version} from {src}" if src else version


def own() -> str | None:
    """The running pm's direct_url.json; None when it has none."""
    try:
        return importlib.metadata.distribution(NAME).read_text("direct_url.json")
    except importlib.metadata.PackageNotFoundError:
        return None


def running() -> str:
    """The running pm's build, which the service sends and every probe compares."""
    return build(__version__, own())


def installed(py: Path) -> tuple[str, str | None] | None:
    """The version and direct_url.json of the pm `py` runs; None when it is missing or runs no pm."""
    if not py.exists():
        return None
    res = subprocess.run([str(py), "-c", PROBE], capture_output=True, text=True)
    return tuple(json.loads(res.stdout)) if res.returncode == 0 else None


def path() -> str:
    """$PATH without the bin dir of the environment this pm runs from, unless that is the tool's: uvx puts its
    ephemeral environment first, and neither a `pm` found there nor a unit's PATH may depend on it."""
    own = (Path(sys.prefix) / "bin").resolve()
    if own == python().parent.resolve():
        return os.environ.get("PATH", "")
    return os.pathsep.join(d for d in os.environ.get("PATH", "").split(os.pathsep) if d and Path(d).resolve() != own)


def current() -> Path:
    """The tool's interpreter, refused unless the tool runs this pm's build, and unless this pm came from git."""
    cmd = install_command()
    py = python()
    have = installed(py)
    if launched():  # the tool launched this pm for the repo's pin: any launcher runs the pin
        if have is None or old(have[0]):
            what = f"runs pm {shown(*have)}" if have else "is not installed"
            first = ".".join(map(str, FIRST))
            raise RecordError(f"the pm uv tool ({py}) {what}, which cannot run this repo's pin {__version__}; install "
                              f"a pm uv tool at {first} or later, such as {INSTALL.format(v=__version__)}")
        return py
    if have is None or build(*have) != running():
        what = f"runs pm {shown(*have)}" if have else "is not installed"
        raise RecordError(f"the pm uv tool ({py}) {what}, not pm {shown(__version__, own())}; run pm init, or "
                          f"install it with {cmd}")
    return py


def source() -> str:
    """The requirement that installs the running pm again, from its direct_url.json: its git commit. Refused when it
    came from elsewhere (a local checkout, an editable install, an index): it cannot install the tool, and the tool,
    installed from git, never runs its build, so nothing that installs or checks the tool or the service may run."""
    src = origin(own())
    if src is None or not src.startswith("git+"):
        release = RELEASE.format(v=__version__)
        raise RecordError(f"pm {__version__} here runs from {src or 'an install with no PEP 610 source'}, not from "
                          "git, and a local checkout cannot install or check the pm uv tool; run pm as the tool "
                          f"({INSTALL.format(v=__version__)}, then pm init), or once with uvx --from \"{release}\" "
                          "pm init")
    return src


def install_command() -> str:
    """The command that installs this pm's build as the tool, from its git commit; refused as source() is."""
    return f'uv tool install --reinstall "{source()}"'


def ensure() -> str:
    """Install the pm uv tool at this build when it runs another or none, from this pm's source, and check that
    `pm` on PATH is the tool's, which hooks call; empty when it was already current. Refused when the installed
    tool's source cannot be read (an install from an index), since then pm cannot tell which build it runs, and
    when this pm did not come from git (source())."""
    if launched():
        current()
        return (f"left the pm uv tool ({python()}) as it is: it launched this pm {__version__} for the repo's pin, "
                "and installing this version would replace the launcher")
    said = ""
    spec = source()
    py = python()
    have = installed(py)
    if have is not None and origin(have[1]) is None:
        raise RecordError(f"the pm uv tool ({py}) runs pm {have[0]} with no source pm can read (no PEP 610 "
                          f"direct_url.json), so pm cannot tell its build; replace it with {install_command()}, "
                          "then run pm init again")
    if have is None or build(*have) != running():
        uv("tool", "install", "--reinstall", spec)
        have = installed(py)
        if have is None or build(*have) != running():
            raise RecordError(f"uv tool install --reinstall {spec} left {py} running pm "
                              f"{shown(*have) if have else 'nothing'}, not {shown(__version__, own())}")
        said = f"installed the pm uv tool {__version__} from {spec}"
    want = Path(uv("tool", "dir", "--bin")) / NAME
    found = shutil.which(NAME, path=path())
    if found is None or Path(found).resolve() != want.resolve():
        raise RecordError(f"`pm` on PATH is {found or 'missing'}, not the pm uv tool's {want}, and pm's hooks run "
                          f"`pm`; put {want.parent} first on PATH (uv tool update-shell), then run pm init again")
    return said
