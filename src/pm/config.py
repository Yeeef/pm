"""The repo's pm settings: `.pm/config.toml` in the main checkout, tracked, pinning the pm version every session runs.

Every pm command reads it first and fails hard when it is missing, malformed, or pins a version other than the one
running; nothing falls back to a default. The pm uv tool runs the pinned version itself (launch.py), so this check
fails only when that launch did not give the pinned version."""

from __future__ import annotations

import functools
import re
import subprocess
import tomllib
from dataclasses import dataclass
from pathlib import Path

from pm import __version__

REL = ".pm/config.toml"
STORE = ".pm/store/records"  # the records store, under the main checkout
RUN = ".pm/run"  # runtime state in the main checkout, never committed: the service log, the push state, locks
REPO = "https://github.com/Yeeef/yeeef-agents"  # where pm's releases are: tag pm-v<version>, the package in pm/
RELEASE = f"git+{REPO}@pm-v{{v}}#subdirectory=pm"  # a release's requirement
INSTALL = f'uv tool install "{RELEASE}"'
KEYS = {"version": str, "remote": str, "main_branch": str, "port": int, "site_url": str}
REQUIRED = ("version", "remote", "main_branch", "port")


class ConfigError(Exception):
    """The repo's config is missing, malformed, or pins another pm version."""


@dataclass(frozen=True)
class Config:
    path: Path
    version: str
    remote: str
    main_branch: str
    port: int
    site_url: str  # "" when unset: links use http://localhost:<port>


def root(cwd: Path) -> Path:
    """The checkout whose config applies, as store.code_root picks the worktree a command acts on: cwd's worktree,
    or the main checkout from inside the store (the records branch carries no config). A worktree reads its own
    branch's config, so the pin moves with the code it pins."""
    try:
        res = subprocess.run(["git", "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-common-dir"],
                             cwd=cwd, capture_output=True, text=True)
    except OSError as e:
        raise ConfigError(f"git did not run, so pm cannot find this repo's {REL}: {e}")
    if res.returncode != 0:
        raise ConfigError(f"{cwd} is not in a git worktree: {(res.stderr or res.stdout).strip()}")
    top, common = (Path(p).resolve() for p in res.stdout.split("\n")[:2])
    return common.parent if common.name == ".git" and top == common.parent / STORE else top


@functools.cache
def load(cwd: Path) -> Config:
    """The config of the checkout containing `cwd`, checked against the running pm's version."""
    c = read(cwd)
    if c.version != __version__:
        raise ConfigError(f"this repo pins pm {c.version} in {c.path}, but pm {__version__} is running, launched "
                          f"for that pin: release tag pm-v{c.version} at {REPO} builds pm {__version__}; fix the tag, "
                          f"or move the pin to {__version__} with pm upgrade")
    return c


def read(cwd: Path) -> Config:
    """The config of the checkout containing `cwd`, checked for its keys but not its pin: `pm upgrade` moves it."""
    path = root(cwd) / REL
    if not path.is_file():
        raise ConfigError(f"this repo has no {REL} (looked for {path}); create it with pm init")
    try:
        data = tomllib.loads(path.read_text())
    except tomllib.TOMLDecodeError as e:
        raise ConfigError(f"{path} is not valid TOML: {e}")
    unknown = sorted(set(data) - set(KEYS))
    missing = [k for k in REQUIRED if k not in data]
    wrong = [k for k, v in data.items() if k in KEYS and type(v) is not KEYS[k]]
    if unknown or missing or wrong:
        raise ConfigError(f"{path}: " + "; ".join(filter(None, [
            unknown and f"unknown keys {', '.join(unknown)}", missing and f"missing keys {', '.join(missing)}",
            wrong and "wrong types for " + ", ".join(f"{k} (want {KEYS[k].__name__})" for k in wrong)])))
    return Config(path, data["version"], data["remote"], data["main_branch"], data["port"],
                  data.get("site_url", "").strip().rstrip("/"))


def run_dir(main: Path) -> Path:
    """The clone's runtime-state directory, `<main checkout>/.pm/run`; writers create it."""
    return main / RUN


def write_site_url(cfg: Config, url: str) -> None:
    """Set `site_url` in the config file ("" removes it), leaving every other line as it is."""
    text = cfg.path.read_text()
    line = re.compile(r"^site_url\s*=.*\n?", re.M)
    new = f'site_url = "{url}"\n' if url else ""
    if line.search(text):
        text = line.sub(new, text, count=1)
    elif url:
        text += ("" if text.endswith("\n") or not text else "\n") + new
    cfg.path.write_text(text)
    load.cache_clear()
