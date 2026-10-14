#!/usr/bin/env python3
"""A stand-in for Codex in tests.

`codex --version` says 0.154.0, a codex with codex queue, which pm doctor checks the pm service has.

`codex app-server`, which pm doctor and pm init ask whether Codex runs pm's hooks: it answers initialize, then
hooks/list with each command hook of <cwd>/.codex/hooks.json in Codex 0.154's shape, every one trusted unless
$FAKE_CODEX_TRUST names a JSON file mapping "<event label>:<group>:<handler>" to another trust status;
{"project": "untrusted"} there lists no hook, as Codex does for a project the user has not trusted. Each start appends
a line to $FAKE_CODEX_APP_LOG.

`codex exec`, which pm's owner-request judge and pm day summarize run: appends its arguments, stdin, working directory,
the files there, its model instructions file's text, $CODEX_HOME and its pid to $FAKE_CODEX_LOG as one JSON line,
prints a line of progress, sleeps $FAKE_CODEX_SLEEP seconds when set, then writes $FAKE_CODEX_OUTPUT when it is set,
else "Summary <n>." (n counting its calls), to its --output-last-message file; or, when $FAKE_CODEX_FAIL is set, fails
with that text on stderr.

`codex queue --thread T --message M`, which the pm service runs to hand a Codex session the owner's answer: appends
{"queue": its arguments, "codex_home": $CODEX_HOME} to $FAKE_CODEX_LOG as one JSON line and says it queued the message
for thread T, as codex 0.154 does."""

import json
import os
import sys
import time
from pathlib import Path

LABEL = {"PreToolUse": "pre_tool_use", "PermissionRequest": "permission_request", "PostToolUse": "post_tool_use",
         "PreCompact": "pre_compact", "PostCompact": "post_compact", "SessionStart": "session_start",
         "SessionEnd": "session_end", "UserPromptSubmit": "user_prompt_submit", "SubagentStart": "subagent_start",
         "SubagentStop": "subagent_stop", "Stop": "stop", "Interrupt": "interrupt"}

if sys.argv[1:] == ["--version"]:  # a codex with codex queue, so the pm service's codex is current
    print("codex-cli 0.154.0")
    sys.exit(0)
if sys.argv[1:2] == ["exec"]:
    args = sys.argv[2:]
    instructions = next((a.split("=", 1)[1] for a in args if a.startswith("model_instructions_file=")), None)
    log = os.environ["FAKE_CODEX_LOG"]
    with open(log, "a") as f:
        f.write(json.dumps({"args": args, "stdin": sys.stdin.read(), "cwd": os.getcwd(), "files": sorted(os.listdir()),
                            "instructions": Path(json.loads(instructions)).read_text() if instructions else None,
                            "codex_home": os.environ.get("CODEX_HOME"), "pid": os.getpid()}) + "\n")
    print("codex: working")
    if os.environ.get("FAKE_CODEX_SLEEP"):  # a judge still thinking when its hook is stopped
        time.sleep(float(os.environ["FAKE_CODEX_SLEEP"]))  # noqa: TID251 — the fake judge thinking, which its hook stops
    if os.environ.get("FAKE_CODEX_FAIL"):
        print(os.environ["FAKE_CODEX_FAIL"], file=sys.stderr)
        sys.exit(1)
    out = args[args.index("--output-last-message") + 1]
    Path(out).write_text(os.environ.get("FAKE_CODEX_OUTPUT") or f"Summary {len(Path(log).read_text().splitlines())}.")
    sys.exit(0)
if sys.argv[1:2] == ["queue"]:
    args = sys.argv[2:]
    with open(os.environ["FAKE_CODEX_LOG"], "a") as f:
        f.write(json.dumps({"queue": args, "codex_home": os.environ.get("CODEX_HOME")}) + "\n")
    print(f"Queued message 01a1-q for thread {args[args.index('--thread') + 1]}.")
    sys.exit(0)
if sys.argv[1:] != ["app-server"]:
    sys.exit(f"fake codex: only --version, app-server, exec and queue, not {sys.argv[1:]}")
if os.environ.get("FAKE_CODEX_APP_LOG"):
    with open(os.environ["FAKE_CODEX_APP_LOG"], "a") as f:
        f.write(json.dumps({"cwd": os.getcwd()}) + "\n")
trust = json.loads(Path(os.environ["FAKE_CODEX_TRUST"]).read_text()) if os.environ.get("FAKE_CODEX_TRUST") else {}


def answer(msg):
    print(json.dumps(msg), flush=True)


for line in sys.stdin:
    req = json.loads(line)
    if req.get("method") == "initialize":
        answer({"id": req["id"], "result": {"userAgent": "fake"}})
    elif req.get("method") == "hooks/list":
        data = []
        for cwd in req["params"]["cwds"]:
            path = Path(cwd) / ".codex/hooks.json"
            hooks = []
            if trust.get("project") != "untrusted" and path.exists():
                for event, groups in json.loads(path.read_text())["hooks"].items():
                    for gi, g in enumerate(groups):
                        for hi, h in enumerate(g["hooks"]):
                            key = f"{LABEL[event]}:{gi}:{hi}"
                            hooks.append({"key": f"{path}:{key}", "eventName": event[0].lower() + event[1:],
                                          "handlerType": "command", "command": h["command"], "sourcePath": str(path),
                                          "source": "project", "enabled": True,
                                          "trustStatus": trust.get(key, "trusted")})
            data.append({"cwd": cwd, "hooks": hooks, "warnings": [], "errors": []})
        answer({"id": req["id"], "result": {"data": data}})
        break
