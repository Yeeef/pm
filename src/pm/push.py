"""The scheduled push: one job per clone pushes Beads data (`bd dolt push`) and the records branch, so no session
pushes either. `pm push` is the job; `pm setup` installs its schedule (launchd, a systemd user timer, or cron).

Each run records the outcome of each store's last attempt in a state file in the main checkout's .pm/run, outside
any branch, and appends to a log beside it (pm push is the log's only writer of result lines; the schedulers send only a
crash's stderr there); `pm show`, `pm where` and the served site read the state to flag a failed or overdue push.
"""

from __future__ import annotations

import fcntl
import hashlib
import html
import json
import os
import plistlib
import shlex
import shutil
import signal
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path

from .config import RUN
from .records import RecordError

INTERVAL = 600            # seconds between scheduled runs
OVERDUE = 3 * INTERVAL    # no successful push for this long is flagged
TIMEOUT = 120             # seconds each step (bd dolt push, git fetch, rebase, push) may take
STORES = ("beads", "records")
STEPS = ("beads", "summary", "records")  # the job's steps in order: summary is pm day summarize, before the records push
BRANCH = "records"


def files(main: Path) -> tuple[Path, Path, Path]:
    """The state file, the log and the lock file, in the main checkout's .pm/run (git-ignored by .pm/.gitignore)."""
    run = main / RUN
    return run / "push.json", run / "push.log", run / "push.lock"


def now() -> datetime:
    return datetime.now(timezone.utc).replace(microsecond=0)


def read_state(main: Path) -> dict:
    path = files(main)[0]
    return json.loads(path.read_text()) if path.exists() else {}


def write_state(main: Path, state: dict) -> None:
    path = files(main)[0]
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_suffix(".tmp")
    tmp.write_text(json.dumps(state, indent=1) + "\n")
    tmp.replace(path)


def run(cmd: list[str], cwd: Path) -> tuple[bool, str]:
    """Run a step with the timeout; ok and its output (or why it failed), on one line. The step runs in its own
    process group and a timeout kills the whole group: git and bd start ssh, git-remote-https or dolt children that
    would outlive a kill of the direct child."""
    try:
        proc = subprocess.Popen(cmd, cwd=cwd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
                                start_new_session=True)
    except FileNotFoundError:
        return False, f"{cmd[0]} is not installed"
    try:
        out, _ = proc.communicate(timeout=TIMEOUT)
    except subprocess.TimeoutExpired:
        os.killpg(proc.pid, signal.SIGKILL)
        proc.communicate()
        return False, f"{' '.join(cmd)} timed out after {TIMEOUT}s"
    said = " ".join(out.split())
    return proc.returncode == 0, said if proc.returncode == 0 else f"{' '.join(cmd)} failed: {said}"


def push_beads(main: Path) -> tuple[bool, str]:
    return run(["bd", "dolt", "push"], main)


def push_records(store: Path, remote: str) -> tuple[bool, str]:
    """Push the store when it is ahead of <remote>/records, rebasing onto it first when the remote moved. The rebase
    holds the store lock every pm write takes; a rebase that stops is aborted, and an abort that fails or leaves HEAD
    off the records branch is reported as needing repair by hand."""
    ok, said = run(["git", "fetch", "--quiet", remote, BRANCH], store)
    if not ok:
        return False, said
    ok, counts = run(["git", "rev-list", "--left-right", "--count", f"HEAD...{remote}/{BRANCH}"], store)
    if not ok:
        return False, counts
    ahead, behind = map(int, counts.split())
    if not ahead:
        return True, f"up to date with {remote}/{BRANCH}" + (f" ({behind} behind; not pulled)" if behind else "")
    if behind:
        fd = os.open(store, os.O_RDONLY)
        try:
            fcntl.flock(fd, fcntl.LOCK_EX)
            if not run(["git", "diff", "--quiet", "HEAD"], store)[0]:
                return False, (f"{remote}/{BRANCH} moved and the store has uncommitted changes to tracked records; "
                               "not rebased, retried on the next run")
            ok, said = run(["git", "rebase", "--quiet", f"{remote}/{BRANCH}"], store)
            if not ok:
                aborted, why = run(["git", "rebase", "--abort"], store)
                on, branch = run(["git", "symbolic-ref", "--short", "HEAD"], store)
                if not aborted or not on or branch != BRANCH:
                    return False, (f"rebase onto {remote}/{BRANCH} stopped and its abort did not restore the store, "
                                   f"which needs manual repair: git -C {store} status, then git -C {store} rebase "
                                   f"--abort or git -C {store} switch {BRANCH} (abort: {why or 'ok'}; HEAD: "
                                   f"{branch or 'detached'}); the rebase: {said}")
                return False, f"rebase onto {remote}/{BRANCH} stopped, aborted and the store left as it was: {said}"
        finally:
            os.close(fd)
    ok, said = run(["git", "push", "--quiet", remote, f"HEAD:{BRANCH}"], store)
    if not ok:
        return False, said
    return True, f"pushed {ahead} commit(s)" + (f" after rebasing onto {behind} new on {remote}/{BRANCH}" if behind else "")


def push(main: Path, remote: str, find, summarize) -> tuple[int, str]:
    """One run of the job, under a lock no second run waits for: push Beads, summarize today (`summarize` returns ok
    and what it did, or raises), push the records. `find` returns the store, so a store that cannot be found is that
    step's recorded failure, as is any step that raises; a failed step does not stop the next."""
    log, lock = files(main)[1:]
    lock.parent.mkdir(parents=True, exist_ok=True)
    fd = os.open(lock, os.O_RDWR | os.O_CREAT, 0o600)
    try:
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            return 0, "another pm push holds the lock; skipped"
        state = read_state(main)
        lines = []
        for name, step in (("beads", lambda: push_beads(main)), ("summary", summarize),
                           ("records", lambda: push_records(find(), remote))):
            at = now().isoformat()
            try:
                ok, said = step()
            except Exception as e:
                ok, said = False, f"{type(e).__name__}: {e}"
            prev = state.get(name, {})
            state[name] = {"at": at, "ok": ok, "message": said, "last_ok": at if ok else prev.get("last_ok"),
                           "first": prev.get("first", at)}
            lines.append(f"{at} {name} {'ok' if ok else 'error'}: {said}")
        write_state(main, state)
        with open(log, "a") as f:
            f.write("".join(l + "\n" for l in lines))
        return (0 if all(state[n]["ok"] for n in STEPS) else 1), "\n".join(lines)
    finally:
        os.close(fd)


def unpushed(store: Path, remote: str) -> int | None:
    """Records commits not on <remote>/records, as of the last fetch; None without it."""
    res = subprocess.run(["git", "rev-list", "--count", f"{remote}/{BRANCH}..HEAD"], cwd=store, capture_output=True,
                         text=True)
    return int(res.stdout) if res.returncode == 0 else None


def flags(main: Path, store: Path | None, remote: str) -> list[str]:
    """A line per step (each store's push, and the day summary) whose last run failed or whose last success is older
    than OVERDUE, counted from the schedule's install when it never succeeded; none before both."""
    state = read_state(main)
    log = files(main)[1]
    out = []
    for name in STEPS:
        kind_ = "push" if name in STORES else "step"
        s = state.get(name)
        if not s:
            if "installed_at" not in state:
                continue
            s = {"ok": True, "last_ok": None, "first": state["installed_at"]}
        if not s["ok"]:
            line = f"{name} {kind_} failed at {s['at']}: {s['message']}"
        else:
            since = s["last_ok"] or s["first"]
            age = (now() - datetime.fromisoformat(since)).total_seconds()
            if age <= OVERDUE:
                continue
            what = f"last successful {kind_}" if s["last_ok"] else f"no successful {kind_} since"
            line = (f"{name} {kind_} overdue: {what} {since}, {int(age // 60)} min ago "
                    f"(the job runs every {INTERVAL // 60} min)")
        if name == "records" and store is not None and (n := unpushed(store, remote)):
            line += f"; {n} records commit(s) not on {remote}/{BRANCH}"
        out.append(line)
    return [f"{l}; log {log}" for l in out]


def banner(main: Path, store: Path, remote: str) -> str:
    """The site's warning over the home and project pages; empty when every push is current."""
    lines = flags(main, store, remote)
    if not lines:
        return ""
    items = "".join(f"<li>{html.escape(l)}</li>" for l in lines)
    return f'<div class="note draft push"><p><strong>Push</strong> — needs attention:</p><ul>{items}</ul></div>'


def describe(main: Path) -> list[str]:
    """`pm where` lines: the last attempt of each store."""
    state = read_state(main)
    log = files(main)[1]
    if not any(n in state for n in STEPS):
        return [f"push      {log}  no push recorded yet"]
    return [f"push      {name} {'ok' if s['ok'] else 'error'} at {s['at']}: {s['message']}"
            for name, s in state.items() if name in STEPS] + [f"push      log {log}"]


# ---------------------------------------------------------------- the schedule

def label(main: Path) -> str:
    """One schedule per clone: its directory name and a hash of its path."""
    digest = hashlib.sha1(str(main.resolve()).encode()).hexdigest()[:8]
    return f"local.pm-push.{main.name}.{digest}"


def quiet(cmd: list[str], input: str | None = None) -> subprocess.CompletedProcess:
    return subprocess.run(cmd, input=input, capture_output=True, text=True)


def has_systemd() -> bool:
    try:
        return quiet(["systemctl", "--user", "show-environment"]).returncode == 0
    except FileNotFoundError:
        return False


def launchd_plist(main: Path) -> Path:
    return Path.home() / "Library/LaunchAgents" / f"{label(main)}.plist"


def systemd_units(main: Path) -> tuple[Path, Path]:
    d = Path(os.environ.get("XDG_CONFIG_HOME") or Path.home() / ".config") / "systemd/user"
    return d / f"{label(main)}.service", d / f"{label(main)}.timer"


def cron_lines() -> list[str]:
    """The user's crontab. Only "no crontab for <user>" counts as empty; any other failure is refused, so an
    install never overwrites a crontab it could not read."""
    try:
        res = quiet(["crontab", "-l"])
    except FileNotFoundError:
        raise RecordError("no scheduler for pm push: systemd has no user instance and crontab is not installed")
    if res.returncode == 0:
        return res.stdout.splitlines()
    if "no crontab for" in res.stderr:
        return []
    raise RecordError(f"crontab -l failed: {(res.stderr or res.stdout).strip()}")


def installed(main: Path) -> tuple[str, bool]:
    """The scheduler holding this clone's job and whether it is installed (and loaded; for systemd, its timer active). On Linux the systemd unit
    and the crontab entry are both checked, so a job under one is never installed again under the other."""
    name = label(main)
    if sys.platform == "darwin":
        return "launchd", launchd_plist(main).exists() and quiet(
            ["launchctl", "print", f"gui/{os.getuid()}/{name}"]).returncode == 0
    if systemd_units(main)[1].exists():
        return "systemd", quiet(["systemctl", "--user", "is-active", f"{name}.timer"]).returncode == 0
    systemd = has_systemd()
    try:
        if any(l.endswith(f"# {name}") for l in cron_lines()):
            return "cron", True
    except RecordError:
        if not systemd:
            raise
    return ("systemd" if systemd else "cron"), False


def launchd_job(main: Path, path: str) -> dict:
    """The launchd agent. pm push writes its own log lines; only stderr (a failure before pm runs) goes to the log."""
    return {"Label": label(main), "ProgramArguments": [str(main / "bin/pm"), "push"], "WorkingDirectory": str(main),
            "StartInterval": INTERVAL, "RunAtLoad": True, "StandardOutPath": "/dev/null",
            "StandardErrorPath": str(files(main)[1]), "EnvironmentVariables": {"PATH": path}}


def unit_quote(value: str) -> str:
    """A systemd unit value, double-quoted, with % (which starts a specifier) escaped."""
    return '"' + value.replace("\\", "\\\\").replace('"', '\\"').replace("%", "%%") + '"'


def install(main: Path) -> str:
    """Install this clone's schedule if it is missing; empty when it is installed. The job runs `<main>/bin/pm push`
    from the main checkout with the PATH setup ran under, refused unless uv, bd and git resolve in it."""
    kind, done = installed(main)
    if done:
        return ""
    name, log, pm = label(main), files(main)[1], main / "bin/pm"
    log.parent.mkdir(parents=True, exist_ok=True)  # the schedulers append a crash's stderr to it
    path = os.environ.get("PATH", "")
    missing = [t for t in ("uv", "bd", "git") if shutil.which(t, path=path) is None]
    if missing:
        raise RecordError(f"the push schedule needs uv, bd, git on PATH; {', '.join(missing)} not found in {path}")
    if kind == "launchd":
        plist = launchd_plist(main)
        if not plist.exists():
            plist.parent.mkdir(parents=True, exist_ok=True)
            plist.write_bytes(plistlib.dumps(launchd_job(main, path)))
        res = quiet(["launchctl", "bootstrap", f"gui/{os.getuid()}", str(plist)])
        if res.returncode != 0:
            raise RecordError(f"launchctl bootstrap {plist} failed: {(res.stderr or res.stdout).strip()}")
        said = f"launchd agent {name} ({plist})"
    elif kind == "systemd":
        service, timer = systemd_units(main)
        service.parent.mkdir(parents=True, exist_ok=True)
        # WorkingDirectory= takes the rest of the line as the path: quotes would be part of it, and systemd then
        # refuses the unit as not absolute.
        service.write_text(f"[Unit]\nDescription=pm push\n\n[Service]\nType=oneshot\n"
                           f"WorkingDirectory={str(main).replace('%', '%%')}\nEnvironment={unit_quote('PATH=' + path)}\n"
                           f"ExecStart={unit_quote(str(pm))} push\nStandardOutput=null\n"
                           f"StandardError=append:{str(log).replace('%', '%%')}\n")
        timer.write_text(f"[Unit]\nDescription=pm push every {INTERVAL // 60} min\n\n[Timer]\n"
                         f"OnBootSec=1min\nOnUnitActiveSec={INTERVAL}s\n\n[Install]\nWantedBy=timers.target\n")
        for cmd in (["systemctl", "--user", "daemon-reload"], ["systemctl", "--user", "enable", "--now", timer.name]):
            res = quiet(cmd)
            if res.returncode != 0:
                raise RecordError(f"{' '.join(cmd)} failed: {(res.stderr or res.stdout).strip()}")
        said = f"systemd user timer {timer}"
    else:
        line = (f"*/{INTERVAL // 60} * * * * cd {shlex.quote(str(main))} && PATH={shlex.quote(path)} "
                f"{shlex.quote(str(pm))} push > /dev/null 2>>{shlex.quote(str(log))} # {name}")
        res = quiet(["crontab", "-"], input="".join(l + "\n" for l in cron_lines() + [line]))
        if res.returncode != 0:
            raise RecordError(f"crontab - failed: {(res.stderr or res.stdout).strip()}")
        said = f"crontab entry {name}"
    write_state(main, {**read_state(main), "installed_at": now().isoformat()})
    return f"installed the push schedule: {said}, every {INTERVAL // 60} min"


def where(main: Path) -> str:
    kind, done = installed(main)
    at = {"launchd": launchd_plist(main), "systemd": systemd_units(main)[1], "cron": "crontab"}[kind]
    return f"schedule  {kind} {label(main)} ({at})  " + ("installed" if done else "missing; run bin/pm setup")
