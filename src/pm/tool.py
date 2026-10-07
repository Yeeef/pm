"""The pm uv tool: the one pm a machine runs, which `pm init` installs and the pm service's unit runs.

`uvx --from "git+…@pm-v<X>#subdirectory=pm" pm init` runs pm from an ephemeral uv cache environment, which
`uv cache clean` removes; the service and the hooks need a pm that stays. So `pm init` installs the tool from the
source the running pm came from (PEP 610's direct_url.json: the git URL and commit), unless the tool already runs
this version, and refuses, naming the command, when it cannot: a pm from a local directory or an index has no
source to install from. The service's unit runs the tool's interpreter, whose path stays the same across versions."""

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


def version(py: Path) -> str | None:
    """The pm version `py` runs; None when it is missing or runs no pm."""
    if not py.exists():
        return None
    res = subprocess.run([str(py), "-c", "import pm; print(pm.__version__)"], capture_output=True, text=True)
    return res.stdout.strip() if res.returncode == 0 else None


def path() -> str:
    """$PATH without the bin dir of the environment this pm runs from, unless that is the tool's: uvx puts its
    ephemeral environment first, and neither a `pm` found there nor a unit's PATH may depend on it."""
    own = (Path(sys.prefix) / "bin").resolve()
    if own == python().parent.resolve():
        return os.environ.get("PATH", "")
    return os.pathsep.join(d for d in os.environ.get("PATH", "").split(os.pathsep) if d and Path(d).resolve() != own)


def current() -> Path:
    """The tool's interpreter, refused unless the tool runs this pm's version."""
    py = python()
    have = version(py)
    if have != __version__:
        what = f"runs pm {have}" if have else "is not installed"
        raise RecordError(f"the pm uv tool ({py}) {what}, not pm {__version__}; run pm init, or install it with "
                          f"{INSTALL.format(v=__version__)}")
    return py


def source() -> str | None:
    """The requirement that installs the running pm again, from its direct_url.json; None unless it came from git."""
    try:
        raw = importlib.metadata.distribution(NAME).read_text("direct_url.json")
    except importlib.metadata.PackageNotFoundError:
        return None
    info = json.loads(raw) if raw else {}
    vcs = info.get("vcs_info") or {}
    if vcs.get("vcs") != "git" or not vcs.get("commit_id"):
        return None
    sub = f"#subdirectory={info['subdirectory']}" if info.get("subdirectory") else ""
    return f"git+{info['url']}@{vcs['commit_id']}{sub}"


def ensure() -> str:
    """Install the pm uv tool at this version when it runs another or none, from this pm's source, and check that
    `pm` on PATH is the tool's, which hooks call; empty when it was already current."""
    said = ""
    py = python()
    if version(py) != __version__:
        spec = source()
        if spec is None:
            raise RecordError(f"pm init installs the pm uv tool from the source this pm runs from, but pm "
                              f"{__version__} here was not installed from git; install the tool with "
                              f"{INSTALL.format(v=__version__)}, then run pm init again")
        uv("tool", "install", spec)
        if version(py) != __version__:
            raise RecordError(f"uv tool install {spec} left {py} running pm {version(py)}, not {__version__}")
        said = f"installed the pm uv tool {__version__} from {spec}"
    want = Path(uv("tool", "dir", "--bin")) / NAME
    found = shutil.which(NAME, path=path())
    if found is None or Path(found).resolve() != want.resolve():
        raise RecordError(f"`pm` on PATH is {found or 'missing'}, not the pm uv tool's {want}, and pm's hooks run "
                          f"`pm`; put {want.parent} first on PATH (uv tool update-shell), then run pm init again")
    return said
