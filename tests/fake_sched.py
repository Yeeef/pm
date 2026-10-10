#!/usr/bin/env python3
"""A stand-in for launchctl, systemctl and crontab in tests, by the name it is called as: logs each call to
$FAKE_SCHED_LOG and keeps what is loaded, the pid each loaded unit runs as, and each unit's enabled state (systemd's
enabled units, launchd's disabled labels) in $FAKE_SCHED_STATE (a JSON object). Loading a unit starts its command as the supervisor would (working directory, environment, log), so the
service really answers; unloading it stops the process. Nothing restarts a process that exits. No test touches the
user's real services; conftest stops every process left at a test's end."""

import json
import os
import re
import signal
import subprocess
import sys
import time
import plistlib
from pathlib import Path

tool, args = os.environ["FAKE_TOOL"], sys.argv[1:]
log, state_path = os.environ["FAKE_SCHED_LOG"], Path(os.environ["FAKE_SCHED_STATE"])
with open(log, "a") as f:
    f.write(json.dumps([tool, *args]) + "\n")
state = json.loads(state_path.read_text()) if state_path.exists() else {"loaded": []}
state.setdefault("pids", {})
state.setdefault("enabled", [])  # systemd: a unit is disabled until enabled
state.setdefault("disabled", [])  # launchd: a label is enabled until disabled


def mark(key: str, name: str, on: bool) -> None:
    if on and name not in state[key]:
        state[key].append(name)
    elif not on and name in state[key]:
        state[key].remove(name)


def save() -> None:
    state_path.write_text(json.dumps(state))


def unit(name: str) -> str:
    return name.removesuffix(".timer").removesuffix(".service")


def systemd_file(name: str) -> Path:
    return Path(os.environ["XDG_CONFIG_HOME"]) / "systemd/user" / f"{unit(name)}.service"


def job(name: str) -> tuple[list[str], str, dict, str]:
    """The unit's command, working directory, environment and log, read from the file install wrote."""
    if tool == "launchctl":
        plist = plistlib.loads((Path(os.environ["HOME"]) / "Library/LaunchAgents" / f"{name}.plist").read_bytes())
        return (plist["ProgramArguments"], plist["WorkingDirectory"], plist["EnvironmentVariables"],
                plist["StandardOutPath"])
    text = systemd_file(name).read_text()
    field = lambda key: re.search(rf"^{key}=(.*)$", text, re.M).group(1)
    words = lambda line: [json.loads(w).replace("%%", "%") for w in re.findall(r'"(?:[^"\\]|\\.)*"', line)]
    env = dict(w.split("=", 1) for w in words(field("Environment")))
    return (words(field("ExecStart")), field("WorkingDirectory").replace("%%", "%"), env,
            field("StandardOutput").removeprefix("append:").replace("%%", "%"))


def start(name: str) -> None:
    cmd, cwd, env, out = job(name)
    # the repo fixture's per-test pm service (conftest.py, Repo.start_service) holds the clone's work store; the
    # clone's installed service takes it over, as only one process may hold it
    fixture = state["pids"].pop("fixture", None)
    if fixture is not None:
        try:
            os.kill(fixture, signal.SIGTERM)
            deadline = time.monotonic() + 10
            while time.monotonic() < deadline:
                os.kill(fixture, 0)
                time.sleep(0.02)
        except ProcessLookupError:
            pass
    Path(out).parent.mkdir(parents=True, exist_ok=True)
    with open(out, "a") as f:
        proc = subprocess.Popen(cmd, cwd=cwd, env={**os.environ, **env}, stdin=subprocess.DEVNULL, stdout=f,
                                stderr=subprocess.STDOUT, start_new_session=True)
    state["pids"][name] = proc.pid
    if name not in state["loaded"]:
        state["loaded"].append(name)


def stop(name: str) -> None:
    pid = state["pids"].pop(name, None)
    if pid is not None:
        try:
            os.kill(pid, signal.SIGTERM)
            deadline = time.monotonic() + 5
            while time.monotonic() < deadline:  # the process is not this one's child once a call returns
                os.kill(pid, 0)
                time.sleep(0.05)
        except ProcessLookupError:
            pass
    if name in state["loaded"]:
        state["loaded"].remove(name)


if tool == "launchctl" and args[:1] == ["bootstrap"]:
    start(Path(args[2]).stem)
    save()
elif tool == "launchctl" and args[:1] == ["bootout"]:
    stop(args[1].rsplit("/", 1)[1])
    save()
elif tool == "launchctl" and args[:1] in (["disable"], ["enable"]):
    mark("disabled", args[1].rsplit("/", 1)[1], args[0] == "disable")
    save()
elif tool == "launchctl" and args[:1] == ["print-disabled"]:
    print("disabled services = {")
    for name in state["disabled"]:
        print(f'\t"{name}" => disabled')
    print("}")
elif tool == "launchctl" and args[:1] == ["bootstrap"] and Path(args[2]).stem in state["disabled"]:
    print(f"Bootstrap failed: 5: Input/output error", file=sys.stderr)  # as launchd answers for a disabled label
    sys.exit(5)
elif tool == "launchctl" and args[:1] == ["print"]:
    sys.exit(0 if args[1].rsplit("/", 1)[1] in state["loaded"] else 113)
elif tool == "launchctl" and args[:2] == ["kickstart", "-k"]:
    name = args[2].rsplit("/", 1)[1]
    if name not in state["loaded"]:
        sys.exit(113)
    stop(name)
    start(name)
    save()
elif tool == "systemctl" and args[1:2] == ["show-environment"]:
    pass
elif tool == "systemctl" and args[1:2] == ["daemon-reload"]:
    pass
elif tool == "systemctl" and args[1:3] == ["enable", "--now"]:
    mark("enabled", unit(args[3]), True)
    if unit(args[3]) not in state["loaded"]:
        start(unit(args[3]))
    save()
elif tool == "systemctl" and args[1:3] == ["disable", "--now"]:
    mark("enabled", unit(args[3]), False)
    stop(unit(args[3]))
    save()
elif tool == "systemctl" and args[1:2] in (["enable"], ["disable"]):
    mark("enabled", unit(args[2]), args[1] == "enable")
    save()
elif tool == "systemctl" and args[1:2] == ["is-enabled"]:
    on = unit(args[2]) in state["enabled"]
    print("enabled" if on else "disabled")
    sys.exit(0 if on else 1)
elif tool == "systemctl" and args[1:2] == ["restart"]:
    if not systemd_file(args[2]).exists():  # as systemd: restart starts a unit that is not running
        sys.exit(5)
    stop(unit(args[2]))
    start(unit(args[2]))
    save()
elif tool == "systemctl" and args[1:2] == ["is-active"]:
    sys.exit(0 if unit(args[2]) in state["loaded"] else 3)
elif tool == "crontab" and args == ["-l"]:
    if "crontab" not in state:
        print("no crontab for test", file=sys.stderr)
        sys.exit(1)
    print(state["crontab"], end="")
elif tool == "crontab" and args == ["-"]:
    state["crontab"] = sys.stdin.read()
    save()
else:
    print(f"fake {tool}: unsupported {args}", file=sys.stderr)
    sys.exit(1)
