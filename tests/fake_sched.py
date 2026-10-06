#!/usr/bin/env python3
"""A stand-in for launchctl, systemctl and crontab in tests, by the name it is called as: logs each call to
$FAKE_SCHED_LOG and keeps what is loaded in $FAKE_SCHED_STATE (a JSON object), so pm setup never touches the user's
real schedule."""

import json
import os
import sys
from pathlib import Path

tool, args = Path(sys.argv[0]).name, sys.argv[1:]
log, state_path = os.environ["FAKE_SCHED_LOG"], Path(os.environ["FAKE_SCHED_STATE"])
with open(log, "a") as f:
    f.write(json.dumps([tool, *args]) + "\n")
state = json.loads(state_path.read_text()) if state_path.exists() else {"loaded": [], "crontab": ""}


def save() -> None:
    state_path.write_text(json.dumps(state))


if tool == "launchctl" and args[:1] == ["bootstrap"]:
    state["loaded"].append(Path(args[2]).stem)
    save()
elif tool == "launchctl" and args[:1] == ["print"]:
    sys.exit(0 if args[1].rsplit("/", 1)[1] in state["loaded"] else 113)
elif tool == "systemctl" and args[1:2] == ["show-environment"]:
    pass
elif tool == "systemctl" and args[1:2] == ["daemon-reload"]:
    pass
elif tool == "systemctl" and args[1:3] == ["enable", "--now"]:
    state["loaded"].append(args[3].removesuffix(".timer"))
    save()
elif tool == "systemctl" and args[1:2] == ["is-active"]:
    sys.exit(0 if args[2].removesuffix(".timer") in state["loaded"] else 1)
elif tool == "crontab" and args == ["-l"]:
    print(state["crontab"], end="")
elif tool == "crontab" and args == ["-"]:
    state["crontab"] = sys.stdin.read()
    save()
else:
    print(f"fake {tool}: unsupported {args}", file=sys.stderr)
    sys.exit(1)
