#!/usr/bin/env python3
"""A stand-in for codex in tests. `codex --version` says 0.154.0, a codex with codex queue, which pm doctor checks the
pm service has. `codex app-server`, which pm doctor and pm init ask whether Codex runs pm's hooks, answers
initialize, then hooks/list with each command hook of <cwd>/.codex/hooks.json in Codex 0.154's shape, every one
trusted unless $FAKE_CODEX_TRUST names a JSON file mapping "<event label>:<group>:<handler>" to another trust status;
{"project": "untrusted"} there lists no hook, as Codex does for a project the user has not trusted."""

import json
import os
import sys
from pathlib import Path

LABEL = {"PreToolUse": "pre_tool_use", "PermissionRequest": "permission_request", "PostToolUse": "post_tool_use",
         "PreCompact": "pre_compact", "PostCompact": "post_compact", "SessionStart": "session_start",
         "SessionEnd": "session_end", "UserPromptSubmit": "user_prompt_submit", "SubagentStart": "subagent_start",
         "SubagentStop": "subagent_stop", "Stop": "stop", "Interrupt": "interrupt"}

if sys.argv[1:] == ["--version"]:  # a codex with codex queue, so the pm service's codex is current
    print("codex-cli 0.154.0")
    sys.exit(0)
if sys.argv[1:] != ["app-server"]:
    sys.exit(f"fake codex: only --version and app-server, not {sys.argv[1:]}")
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
