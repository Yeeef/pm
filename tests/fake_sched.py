#!/usr/bin/env python3
"""A stand-in for launchctl and systemctl in tests, by the name it is called as: logs each call to
$FAKE_SCHED_LOG and keeps what is loaded in $FAKE_SCHED_STATE (a JSON object), so no test touches the user's real
services."""

import json
import os
import sys
from pathlib import Path

tool, args = Path(sys.argv[0]).name, sys.argv[1:]
log, state_path = os.environ["FAKE_SCHED_LOG"], Path(os.environ["FAKE_SCHED_STATE"])
with open(log, "a") as f:
    f.write(json.dumps([tool, *args]) + "\n")
state = json.loads(state_path.read_text()) if state_path.exists() else {"loaded": []}


def save() -> None:
    state_path.write_text(json.dumps(state))


def unit(name: str) -> str:
    return name.removesuffix(".timer").removesuffix(".service")


if tool == "launchctl" and args[:1] == ["bootstrap"]:
    state["loaded"].append(Path(args[2]).stem)
    save()
elif tool == "launchctl" and args[:1] == ["bootout"]:
    state["loaded"].remove(args[1].rsplit("/", 1)[1])
    save()
elif tool == "launchctl" and args[:1] == ["print"]:
    sys.exit(0 if args[1].rsplit("/", 1)[1] in state["loaded"] else 113)
elif tool == "launchctl" and args[:2] == ["kickstart", "-k"]:
    sys.exit(0 if args[2].rsplit("/", 1)[1] in state["loaded"] else 113)
elif tool == "systemctl" and args[1:2] == ["show-environment"]:
    pass
elif tool == "systemctl" and args[1:2] == ["daemon-reload"]:
    pass
elif tool == "systemctl" and args[1:3] == ["enable", "--now"]:
    state["loaded"].append(unit(args[3]))
    save()
elif tool == "systemctl" and args[1:2] == ["restart"]:
    sys.exit(0 if unit(args[2]) in state["loaded"] else 5)
elif tool == "systemctl" and args[1:2] == ["is-active"]:
    sys.exit(0 if unit(args[2]) in state["loaded"] else 3)
else:
    print(f"fake {tool}: unsupported {args}", file=sys.stderr)
    sys.exit(1)
