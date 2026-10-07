"""The pm service: one supervised background process per clone, `pm service run` in the main checkout, serves the site
live from the store and Beads and pushes Beads data and the records branch every push.INTERVAL seconds.

`pm service install` puts it under the machine's supervisor, which starts it at login and restarts it after a crash:
a launchd agent with KeepAlive on macOS, a systemd user service on Linux. A machine with neither has no service, and
install refuses. The unit runs the pm uv tool's interpreter (`python -m pm.cli service run`; pm init installs the
tool) with the PATH and the site port install ran with, so `PORT=<n> pm service install` gives a second clone of the
repo its own port, which installing again keeps. Install waits for the site to answer and fails when it does not.
The service writes its log to `<main checkout>/.pm/run/service.log`; health is the supervisor holding the unit plus
the site answering on its port for this clone's store."""

from __future__ import annotations

import contextlib
import fcntl
import hashlib
import json
import os
import plistlib
import re
import shutil
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

from . import push, tool
from .config import STORE, run_dir
from .records import RecordError

SERVE_HEADER = "X-PM-Store"  # the service's replies name the store they render, so a probe can check who answers
VERSION_HEADER = "X-PM-Version"  # and the pm build they run (tool.running), so a probe sees a service left on an old one
PROBE_TIMEOUT = 1   # seconds a probe of the site may take; pm where runs one at every session start
RESTART_WAIT = 15   # seconds restart waits for the site to answer
TOOLS = ("bd", "git", "uv")  # what the service runs (uv runs the pinned pm); install refuses a PATH without them


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


def command(py: Path) -> list[str]:
    """What the supervisor runs: the pm uv tool's interpreter `py`, whose path stays across versions and cache
    cleans, unlike uvx's environment."""
    return [str(py), "-m", "pm.cli", "service", "run"]


def launchd_job(main: Path, path: str, port: int, py: Path) -> dict:
    log = str(log_path(main))
    return {"Label": label(main), "ProgramArguments": command(py), "WorkingDirectory": str(main), "RunAtLoad": True,
            "KeepAlive": True, "StandardOutPath": log, "StandardErrorPath": log,
            "EnvironmentVariables": {"PATH": path, "PORT": str(port)}}


def unit_quote(value: str) -> str:
    """A systemd unit value, double-quoted, with % (which starts a specifier) escaped."""
    return '"' + value.replace("\\", "\\\\").replace('"', '\\"').replace("%", "%%") + '"'


def systemd_unit(main: Path, path: str, port: int, py: Path) -> str:
    log = str(log_path(main)).replace("%", "%%")
    # WorkingDirectory= takes the rest of the line as the path: quotes would be part of it, and systemd then refuses
    # the unit as not absolute.
    return (f"[Unit]\nDescription=pm service for {main}\n\n[Service]\nType=simple\n"
            f"WorkingDirectory={str(main).replace('%', '%%')}\n"
            f"Environment={unit_quote('PATH=' + path)} {unit_quote(f'PORT={port}')}\n"
            f"ExecStart={' '.join(unit_quote(c) for c in command(py))}\nRestart=always\nRestartSec=5\n"
            f"StandardOutput=append:{log}\nStandardError=append:{log}\n\n[Install]\nWantedBy=default.target\n")


def unit_bytes(main: Path, kind: str, path: str, port: int, py: Path) -> bytes:
    return (plistlib.dumps(launchd_job(main, path, port, py)) if kind == "launchd"
            else systemd_unit(main, path, port, py).encode())


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


FIRST_PORT = 8000  # a new repo's site port is the first free one from here


def port_free(port: int) -> bool:
    """Whether a server could bind 127.0.0.1:`port` now, as the pm service does: with SO_REUSEADDR (http.server
    sets it), so a port whose last connections linger in TIME_WAIT, as after this clone's service stopped, is free."""
    with socket.socket() as s:
        s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        try:
            s.bind(("127.0.0.1", port))
        except OSError:
            return False
    return True


def unit_ports() -> set[int]:
    """The ports every pm service unit on this machine serves on, this clone's too; a unit that names none is skipped."""
    kind = platform_kind()
    out = set()
    for unit in unit_file(Path("x"), kind).parent.glob("local.pm.*"):
        if not unit.is_file():  # a systemd drop-in directory (local.pm.<name>.service.d), say
            continue
        found = re.search(rb"PORT(?:</key>\s*<string>|=)(\d+)", unit.read_bytes())
        if found:
            out.add(int(found.group(1)))
    return out


def free_port() -> int:
    """The first port from FIRST_PORT up that is free and no pm service unit on this machine names."""
    taken = unit_ports()
    port = FIRST_PORT
    while port in taken or not port_free(port):
        port += 1
    return port


def check_port(main: Path, port: int) -> None:
    """Refuse a site port another server holds. This clone's own pm service on it is fine (pm init run again), and so
    is a port this clone's installed unit serves on where nothing answers: its own service hung, which install
    restarts."""
    if port_free(port):
        return
    served = answering(port)
    if served is not None and served[0] and Path(served[0]) == (main / STORE).resolve():
        return
    if served is None and installed(main) and unit_port(main, platform_kind()) == port:
        return
    what = (f"the pm service of another store ({served[0]})" if served and served[0]
            else "a server that is not pm" if served else "a process that does not answer HTTP")
    raise RecordError(f"the site port :{port} is held by {what}, so the pm service could not serve there; pm init "
                      f"wrote nothing. Run PORT={free_port()} pm init (a free port), or stop what holds :{port}")


def installed(main: Path) -> bool:
    return unit_file(main, platform_kind()).exists()


@contextlib.contextmanager
def install_lock(main: Path):
    """Hold the clone's install lock, `<main checkout>/.pm/run/install.lock`, waiting for another holder: two session
    starts or a typed pm init in parallel would otherwise rewrite and restart the one unit at once."""
    path = run_dir(main) / "install.lock"
    path.parent.mkdir(parents=True, exist_ok=True)
    with open(path, "a") as f:
        fcntl.flock(f, fcntl.LOCK_EX)
        yield


def install(main: Path, port: int) -> str:
    """Install this clone's service, or bring an installed one up to date, start it and wait until the site answers
    for this clone's store; empty when it is installed, current and held by the supervisor. Refused unless the pm
    uv tool runs this version and bd and git resolve on the PATH it runs with; failed when the site does not
    come up (another clone's service on the port, say). One install per clone runs at a time (install_lock)."""
    with install_lock(main):
        return install_locked(main, port)


def install_locked(main: Path, port: int) -> str:
    kind = supervisor()
    py = tool.current()
    path = tool.path()
    missing = [t for t in TOOLS if shutil.which(t, path=path) is None]
    if missing:
        raise RecordError(f"the pm service needs {', '.join(TOOLS)} on PATH; {', '.join(missing)} not found in {path}")
    unit, want = unit_file(main, kind), unit_bytes(main, kind, path, port, py)
    changed = not unit.exists() or unit.read_bytes() != want
    up = loaded(main, kind)
    if not changed and up and answering(port) == (str((main / STORE).resolve()), tool.running()):
        return ""  # a unit held but answering on another build (the tool just moved) or not at all is restarted
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
    wait_up(main, port, f"{'updated' if up else 'installed'} {name}",
            f"if another clone's service holds :{port}, give this clone its own port with PORT=<n> pm service install")
    return (f"{'updated' if up else 'installed'} the pm service: {said}, serving http://localhost:{port} and pushing "
            f"every {push.INTERVAL // 60} min; log {log_path(main)}")


def wait_up(main: Path, port: int, done: str, hint: str = "") -> str:
    """Wait up to RESTART_WAIT seconds for the site to answer on `port` for this clone's store; its health line,
    or failed naming what answers instead, `hint` and the log."""
    deadline = time.monotonic() + RESTART_WAIT
    while True:
        ok, line = health(main, main / STORE)
        if ok:
            return line
        served = answering(port)
        if time.monotonic() > deadline or (served is not None and served[0] != str((main / STORE).resolve())):
            # past the deadline, or another server holds the port, so this clone's service cannot bind it
            raise RecordError(f"{done}, but the site does not answer for this store on :{port} "
                              f"({line.split('  ', 1)[-1].strip()}); {hint + '; ' if hint else ''}read pm service logs")
        time.sleep(0.2)


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
    try:
        want = command(tool.current())
    except RecordError as e:
        out.append(str(e))
    else:
        if unit_command(main, kind) != want:
            out.append(f"{unit} does not run the pm uv tool ({' '.join(want)}); run pm service install")
    with contextlib.suppress(RecordError):
        served = answering(unit_port(main, kind))
        tool.source()  # a pm not from git has said so above and cannot tell a stale service
        if served and Path(served[0]) == (main / STORE).resolve() and served[1] != tool.running():
            out.append(f"the service on :{unit_port(main, kind)} is stale: {stale(served[1])}")
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


def answering(port: int) -> tuple[str, str] | None:
    """The store the server on `port` renders and the pm build it runs, by SERVE_HEADER and VERSION_HEADER ("" for
    one it does not send: what answers is not pm, or a pm too old to say); None when nothing answers. The probe
    fetches style.css, which the service answers without rendering."""
    try:
        with urllib.request.urlopen(f"http://127.0.0.1:{port}/style.css", timeout=PROBE_TIMEOUT) as r:
            headers = r.headers
    except urllib.error.HTTPError as e:
        headers = e.headers
    except OSError:
        return None
    return headers.get(SERVE_HEADER) or "", headers.get(VERSION_HEADER) or ""


def stale(build: str) -> str:
    """What to say of a service answering for this store on another pm build than this one."""
    return (f"it runs pm {build or 'older than 0.1.0'}, not pm {tool.running()}: the pm uv tool is on another build; "
            "run pm init")


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
        want = command(tool.python())
    except RecordError as e:
        return False, head + f"broken: {e}"
    have = unit_command(main, kind)
    if have != want:  # a pin older than the launcher wrote it to run that pin's own tool: restarting runs that again
        return False, head + (f"stale: its unit runs {' '.join(have or ['nothing'])}, not the pm uv tool "
                              f"({' '.join(want)}); run pm service install")
    if not loaded(main, kind):
        return False, head + f"down: {kind} does not hold it; run pm service restart"
    served = answering(port)
    if served is None:
        return False, head + f"down: nothing answers on :{port}; run pm service restart, then pm service logs"
    if Path(served[0]) != store.resolve():
        what = f"another store ({served[0]})" if served[0] else "a server that is not pm"
        return False, head + f"down: :{port} is held by {what}; stop it, then run pm service restart"
    try:
        tool.source()
    except RecordError as e:
        return False, head + f"unchecked: {e}"
    if served[1] != tool.running():
        return False, head + f"stale: {stale(served[1])}"
    return True, head + f"running; the site answers on :{port}"


def status(main: Path, store: Path, remote: str) -> tuple[int, str]:
    """`pm service status`: the service's health and its pushes; non-zero when it is down or a push needs attention."""
    ok, line = health(main, store)
    flags = push.flags(main, store, remote)
    lines = [line, *push.describe(main), *(f"push      needs attention: {f}" for f in flags),
             f"log       {log_path(main)} (pm service logs)"]
    return (0 if ok and not flags else 1), "\n".join(lines)


def restart(main: Path) -> str:
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
    return f"restarted the pm service\n{wait_up(main, unit_port(main, kind), f'restarted {name}')}"


def logs(main: Path, lines: int) -> str:
    """The last `lines` lines of the service log."""
    path = log_path(main)
    if not path.exists():
        raise RecordError(f"no service log at {path}: the service has not run here; run pm service install")
    return "\n".join(path.read_text(errors="replace").splitlines()[-lines:])
