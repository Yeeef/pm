"""The pm uv tool: the one pm a machine runs, which `pm init` installs and the pm service's unit runs.

`uvx --from "git+…@pm-v<X>#subdirectory=pm" pm init` runs pm from an ephemeral uv cache environment, which
`uv cache clean` removes; the service and the hooks need a pm that stays. So `pm init` installs the tool from the
source the running pm came from (PEP 610's direct_url.json: the git URL and commit), unless the tool already runs
this build, and refuses, naming the command, when it cannot: a pm from a local directory or an index has no
source to install from. A build is the version and the source it was installed from, so a tool built from another
commit of the same version is replaced, and the service, which sends its build, is seen as stale. The service's
unit runs the tool's interpreter, whose path stays the same across versions."""

from __future__ import annotations

import importlib.metadata
import json
import os
import shutil
import subprocess
import sys
from pathlib import Path

from pm import __version__
from .config import INSTALL
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
    """A pm build: its version and the source it was installed from, when it names one."""
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
    """The tool's interpreter, refused unless the tool runs this pm's build."""
    py = python()
    have = installed(py)
    if have is None or build(*have) != running():
        what = f"runs pm {build(*have)}" if have else "is not installed"
        raise RecordError(f"the pm uv tool ({py}) {what}, not pm {running()}; run pm init, or install it with "
                          f"{install_command()}")
    return py


def source() -> str | None:
    """The requirement that installs the running pm again, from its direct_url.json; None unless it came from git."""
    src = origin(own())
    return src if src and src.startswith("git+") else None


def install_command() -> str:
    """The command that installs this pm's build as the tool: from its git commit, else its version's tag."""
    spec = source()
    return f'uv tool install --reinstall "{spec}"' if spec else INSTALL.format(v=__version__)


def ensure() -> str:
    """Install the pm uv tool at this build when it runs another or none, from this pm's source, and check that
    `pm` on PATH is the tool's, which hooks call; empty when it was already current. Refused when the installed
    tool's source cannot be read (an install from an index), since then pm cannot tell which build it runs."""
    said = ""
    py = python()
    have = installed(py)
    if have is not None and origin(have[1]) is None:
        raise RecordError(f"the pm uv tool ({py}) runs pm {have[0]} with no source pm can read (no PEP 610 "
                          f"direct_url.json), so pm cannot tell its build; replace it with {install_command()}, "
                          "then run pm init again")
    if have is None or build(*have) != running():
        spec = source()
        if spec is None:
            raise RecordError(f"pm init installs the pm uv tool from the source this pm runs from, but pm "
                              f"{__version__} here was not installed from git; install the tool with "
                              f"{INSTALL.format(v=__version__)}, then run pm init again")
        uv("tool", "install", "--reinstall", spec)
        have = installed(py)
        if have is None or build(*have) != running():
            raise RecordError(f"uv tool install --reinstall {spec} left {py} running pm "
                              f"{build(*have) if have else 'nothing'}, not {running()}")
        said = f"installed the pm uv tool {__version__} from {spec}"
    want = Path(uv("tool", "dir", "--bin")) / NAME
    found = shutil.which(NAME, path=path())
    if found is None or Path(found).resolve() != want.resolve():
        raise RecordError(f"`pm` on PATH is {found or 'missing'}, not the pm uv tool's {want}, and pm's hooks run "
                          f"`pm`; put {want.parent} first on PATH (uv tool update-shell), then run pm init again")
    return said
