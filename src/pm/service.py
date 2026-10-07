"""The pm service: one supervised background process per clone, `pm service run` in the main checkout, serves the site
live from the store and Beads and pushes Beads data and the records branch every push.INTERVAL seconds.

`pm service install` puts it under the machine's supervisor, which starts it at login and restarts it after a crash:
a launchd agent with KeepAlive on macOS, a systemd user service on Linux. A machine with neither has no service, and
install refuses. The unit runs this pm's interpreter (`python -m pm.cli service run`) with the PATH and the site port
install ran with, so `PORT=<n> pm service install` gives a second clone of the repo its own port, which installing
again keeps. The service writes
its log to `<main checkout>/.pm/run/service.log`; health is the supervisor holding the unit plus the site answering
on its port for this clone's store."""

from __future__ import annotations

import hashlib
import json
import os
import plistlib
import re
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

from . import push
from .config import run_dir
from .records import RecordError

SERVE_HEADER = "X-PM-Store"  # the service's replies name the store they render, so a probe can check who answers
PROBE_TIMEOUT = 1   # seconds a probe of the site may take; pm where runs one at every session start
RESTART_WAIT = 15   # seconds restart waits for the site to answer
TOOLS = ("bd", "git")  # what the service runs; install refuses a PATH without them


def label(main: Path) -> str:
    """One service per clone: its directory name and a hash of its path."""
    digest = hashlib.sha1(str(main.resolve()).encode()).hexdigest()[:8]
    return f"local.pm.{main.name}.{digest}"


def log_path(main: Path) -> Path:
    return run_dir(main) / "service.log"


def quiet(cmd: list[str]) -> subprocess.CompletedProcess:
    return subprocess.run(cmd, capture_output=True, text=True)


def checked(cmd: list[str]) -> None:
    res = quiet(cmd)
    if res.returncode != 0:
        raise RecordError(f"{' '.join(cmd)} failed: {(res.stderr or res.stdout).strip()}")


def supervisor() -> str:
    """launchd on macOS, systemd with a user instance elsewhere; refused on a machine with neither."""
    if sys.platform == "darwin":
        return "launchd"
    try:
        if quiet(["systemctl", "--user", "show-environment"]).returncode == 0:
            return "systemd"
    except FileNotFoundError:
        pass
    raise RecordError("no service manager here: the pm service needs launchd (macOS) or a systemd user instance "
                      "(Linux), and this machine has neither")


def platform_kind() -> str:
    """The supervisor this platform's unit is for, without asking it: launchd on macOS, systemd elsewhere."""
    return "launchd" if sys.platform == "darwin" else "systemd"


def unit_file(main: Path, kind: str) -> Path:
    if kind == "launchd":
        return Path.home() / "Library/LaunchAgents" / f"{label(main)}.plist"
    d = Path(os.environ.get("XDG_CONFIG_HOME") or Path.home() / ".config") / "systemd/user"
    return d / f"{label(main)}.service"


def command() -> list[str]:
    """What the supervisor runs: this pm's interpreter, so the service runs the pm that installed it."""
    return [sys.executable, "-m", "pm.cli", "service", "run"]


def launchd_job(main: Path, path: str, port: int) -> dict:
    log = str(log_path(main))
    return {"Label": label(main), "ProgramArguments": command(), "WorkingDirectory": str(main), "RunAtLoad": True,
            "KeepAlive": True, "StandardOutPath": log, "StandardErrorPath": log,
            "EnvironmentVariables": {"PATH": path, "PORT": str(port)}}


def unit_quote(value: str) -> str:
    """A systemd unit value, double-quoted, with % (which starts a specifier) escaped."""
    return '"' + value.replace("\\", "\\\\").replace('"', '\\"').replace("%", "%%") + '"'


def systemd_unit(main: Path, path: str, port: int) -> str:
    log = str(log_path(main)).replace("%", "%%")
    # WorkingDirectory= takes the rest of the line as the path: quotes would be part of it, and systemd then refuses
    # the unit as not absolute.
    return (f"[Unit]\nDescription=pm service for {main}\n\n[Service]\nType=simple\n"
            f"WorkingDirectory={str(main).replace('%', '%%')}\n"
            f"Environment={unit_quote('PATH=' + path)} {unit_quote(f'PORT={port}')}\n"
            f"ExecStart={' '.join(unit_quote(c) for c in command())}\nRestart=always\nRestartSec=5\n"
            f"StandardOutput=append:{log}\nStandardError=append:{log}\n\n[Install]\nWantedBy=default.target\n")


def unit_bytes(main: Path, kind: str, path: str, port: int) -> bytes:
    return (plistlib.dumps(launchd_job(main, path, port)) if kind == "launchd"
            else systemd_unit(main, path, port).encode())


def loaded(main: Path, kind: str) -> bool:
    """Whether the supervisor holds the unit: loaded in launchd, active in systemd."""
    if kind == "launchd":
        return quiet(["launchctl", "print", f"gui/{os.getuid()}/{label(main)}"]).returncode == 0
    return quiet(["systemctl", "--user", "is-active", f"{label(main)}.service"]).returncode == 0


def unit_port(main: Path, kind: str) -> int:
    """The port the installed unit serves on, as install wrote it."""
    path = unit_file(main, kind)
    if kind == "launchd":
        try:
            return int(plistlib.loads(path.read_bytes())["EnvironmentVariables"]["PORT"])
        except (plistlib.InvalidFileException, KeyError, TypeError, ValueError) as e:
            raise RecordError(f"{path} sets no PORT ({type(e).__name__}: {e}); run pm service install again")
    found = re.search(r'"PORT=(\d+)"', path.read_text())
    if not found:
        raise RecordError(f"{path} sets no PORT; run pm service install again")
    return int(found.group(1))


def port_for(main: Path, default: int) -> int:
    """The clone's site port: $PORT, else the installed unit's, so installing again keeps a clone's own port and
    every link and probe uses the port the service serves on, else `default` (the config's)."""
    if os.environ.get("PORT"):
        return int(os.environ["PORT"])
    kind = platform_kind()
    return unit_port(main, kind) if unit_file(main, kind).exists() else default


def installed(main: Path) -> bool:
    return unit_file(main, platform_kind()).exists()


def check_interpreter(main: Path) -> None:
    """Refuse to install a pm that runs from another worktree's environment (bin/pm in a worktree): the unit would
    run that worktree's code, and fail at every start once the worktree is gone."""
    res = subprocess.run(["git", "worktree", "list", "--porcelain"], cwd=main, capture_output=True, text=True)
    prefix = Path(sys.prefix).resolve()
    for line in res.stdout.splitlines():
        tree = Path(line.removeprefix("worktree ")).resolve() if line.startswith("worktree ") else None
        if tree and tree != main.resolve() and prefix.is_relative_to(tree):
            raise RecordError(f"this pm runs from {prefix}, inside the worktree {tree}; the service would run that "
                              f"worktree's code. Run pm service install with the installed pm, or from {main}")


def install(main: Path, port: int) -> str:
    """Install this clone's service, or bring an installed one up to date, and start it; empty when it is installed,
    current and held by the supervisor. Refused unless bd and git resolve on the PATH it runs with."""
    kind = supervisor()
    path = os.environ.get("PATH", "")
    missing = [t for t in TOOLS if shutil.which(t, path=path) is None]
    if missing:
        raise RecordError(f"the pm service needs {', '.join(TOOLS)} on PATH; {', '.join(missing)} not found in {path}")
    check_interpreter(main)
    unit, want = unit_file(main, kind), unit_bytes(main, kind, path, port)
    changed = not unit.exists() or unit.read_bytes() != want
    up = loaded(main, kind)
    if not changed and up:
        return ""
    if changed:
        unit.parent.mkdir(parents=True, exist_ok=True)
        unit.write_bytes(want)
    log_path(main).parent.mkdir(parents=True, exist_ok=True)
    name = label(main)
    if kind == "launchd":
        if up:  # a changed plist takes effect only once launchd loads it again
            checked(["launchctl", "bootout", f"gui/{os.getuid()}/{name}"])
            deadline = time.monotonic() + RESTART_WAIT  # bootout returns before the job is gone; bootstrap fails until then
            while loaded(main, kind):
                if time.monotonic() > deadline:
                    raise RecordError(f"launchd still holds {name} {RESTART_WAIT} s after bootout; run pm service "
                                      "install again")
                time.sleep(0.2)
        checked(["launchctl", "bootstrap", f"gui/{os.getuid()}", str(unit)])
        said = f"launchd agent {name} ({unit})"
    else:
        checked(["systemctl", "--user", "daemon-reload"])
        checked(["systemctl", "--user", "enable", "--now", unit.name] if not up
                else ["systemctl", "--user", "restart", unit.name])
        said = f"systemd user service {name} ({unit})"
    state = push.read_state(main)
    if "installed_at" not in state:  # a push that never succeeds is overdue counted from here
        push.write_state(main, {**state, "installed_at": push.now().isoformat()})
    return (f"{'updated' if up else 'installed'} the pm service: {said}, serving http://localhost:{port} and pushing "
            f"every {push.INTERVAL // 60} min; log {log_path(main)}")


def unit_command(main: Path, kind: str) -> list[str] | None:
    """The command the installed unit runs, as install wrote it; None when it names none."""
    path = unit_file(main, kind)
    if kind == "launchd":
        try:
            args = plistlib.loads(path.read_bytes()).get("ProgramArguments")
        except plistlib.InvalidFileException:
            return None
        return args if isinstance(args, list) else None
    found = re.search(r"^ExecStart=(.*)$", path.read_text(), re.M)
    if not found:
        return None
    return [json.loads(a).replace("%%", "%") for a in re.findall(r'"(?:[^"\\]|\\.)*"', found.group(1))]


def drift(main: Path, port: int) -> list[str]:
    """How this clone's service differs from what pm init installs, one line each: installed, held by the
    supervisor, running this pm, on `port`."""
    try:
        kind = supervisor()
    except RecordError as e:
        return [str(e)]
    unit = unit_file(main, kind)
    if not unit.exists():
        return [f"not installed ({unit} is missing); run pm init"]
    out = []
    if not loaded(main, kind):
        out.append(f"{kind} does not hold {label(main)}; run pm service restart")
    try:
        have = unit_port(main, kind)
    except RecordError as e:
        out.append(str(e))
    else:
        if have != port:
            out.append(f"{unit} serves on :{have}, not :{port}; run pm service install")
    if unit_command(main, kind) != command():
        out.append(f"{unit} does not run this pm ({' '.join(command())}); run pm service install")
    return out


def uninstall(main: Path) -> str:
    """Stop this clone's service and remove its unit; empty when none is installed."""
    kind = platform_kind()
    unit, name = unit_file(main, kind), label(main)
    if not unit.exists():
        return ""
    if loaded(main, kind):
        if kind == "launchd":
            checked(["launchctl", "bootout", f"gui/{os.getuid()}/{name}"])
            deadline = time.monotonic() + RESTART_WAIT
            while loaded(main, kind):
                if time.monotonic() > deadline:
                    raise RecordError(f"launchd still holds {name} {RESTART_WAIT} s after bootout; run pm uninstall again")
                time.sleep(0.2)
        else:
            checked(["systemctl", "--user", "disable", "--now", unit.name])
    unit.unlink()
    if kind == "systemd":
        checked(["systemctl", "--user", "daemon-reload"])
    return f"removed the pm service: {kind} {name} ({unit})"


def answering(port: int) -> str | None:
    """The store the server on `port` renders, by its SERVE_HEADER; "" when what answers is not pm, None when nothing
    answers. The probe fetches style.css, which the service answers without rendering."""
    try:
        with urllib.request.urlopen(f"http://127.0.0.1:{port}/style.css", timeout=PROBE_TIMEOUT) as r:
            return r.headers.get(SERVE_HEADER) or ""
    except urllib.error.HTTPError as e:
        return e.headers.get(SERVE_HEADER) or ""
    except OSError:
        return None


def health(main: Path, store: Path) -> tuple[bool, str]:
    """Whether the service is up, and one `pm where` line saying so or what to run."""
    try:
        kind = supervisor()
    except RecordError as e:
        return False, f"service   {e}"
    name, unit = label(main), unit_file(main, kind)
    head = f"service   {kind} {name} ({unit})  "
    if not unit.exists():
        return False, head + "not installed; run pm service install"
    try:
        port = unit_port(main, kind)
    except RecordError as e:
        return False, head + f"broken: {e}"
    if not loaded(main, kind):
        return False, head + f"down: {kind} does not hold it; run pm service restart"
    served = answering(port)
    if served is None:
        return False, head + f"down: nothing answers on :{port}; run pm service restart, then pm service logs"
    if Path(served) != store.resolve():
        what = f"another store ({served})" if served else "a server that is not pm"
        return False, head + f"down: :{port} is held by {what}; stop it, then run pm service restart"
    return True, head + f"running; the site answers on :{port}"


def status(main: Path, store: Path, remote: str) -> tuple[int, str]:
    """`pm service status`: the service's health and its pushes; non-zero when it is down or a push needs attention."""
    ok, line = health(main, store)
    flags = push.flags(main, store, remote)
    lines = [line, *push.describe(main), *(f"push      needs attention: {f}" for f in flags),
             f"log       {log_path(main)} (pm service logs)"]
    return (0 if ok and not flags else 1), "\n".join(lines)


def restart(main: Path, store: Path) -> str:
    """Restart the installed service (load it when the supervisor does not hold it), then wait for the site to answer
    for this store; refused when it is not installed, failed when it does not come up."""
    kind = supervisor()
    unit, name = unit_file(main, kind), label(main)
    if not unit.exists():
        raise RecordError(f"the pm service is not installed ({unit} is missing); run pm service install")
    if kind == "launchd":
        if loaded(main, kind):
            checked(["launchctl", "kickstart", "-k", f"gui/{os.getuid()}/{name}"])
        else:
            checked(["launchctl", "bootstrap", f"gui/{os.getuid()}", str(unit)])
    else:
        checked(["systemctl", "--user", "restart", unit.name])
    port = unit_port(main, kind)
    deadline = time.monotonic() + RESTART_WAIT
    while time.monotonic() < deadline:
        ok, line = health(main, store)
        if ok:
            return f"restarted the pm service\n{line}"
        time.sleep(0.5)
    raise RecordError(f"restarted {name}, but the site does not answer for this store on :{port} after "
                      f"{RESTART_WAIT} s ({line.split('  ', 1)[-1]}); read pm service logs")


def logs(main: Path, lines: int) -> str:
    """The last `lines` lines of the service log."""
    path = log_path(main)
    if not path.exists():
        raise RecordError(f"no service log at {path}: the service has not run here; run pm service install")
    return "\n".join(path.read_text(errors="replace").splitlines()[-lines:])
