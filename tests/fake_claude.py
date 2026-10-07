#!/usr/bin/env python3
"""A stand-in for `claude -p` in tests: appends its arguments, stdin, working directory and the environment variables
that shape the call to $FAKE_CLAUDE_LOG as one JSON line, then answers $FAKE_CLAUDE_OUTPUT when it is set, else
"Summary <n>." (n counting its calls), or, when $FAKE_CLAUDE_FAIL is set, fails with that text on stderr."""

import json
import os
import sys

log = os.environ["FAKE_CLAUDE_LOG"]
env = {k: os.environ.get(k) for k in ("MAX_THINKING_TOKENS", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC")}
with open(log, "a") as f:
    f.write(json.dumps({"args": sys.argv[1:], "stdin": sys.stdin.read(), "cwd": os.getcwd(), "env": env}) + "\n")
if os.environ.get("FAKE_CLAUDE_FAIL"):
    print(os.environ["FAKE_CLAUDE_FAIL"], file=sys.stderr)
    sys.exit(1)
print(os.environ.get("FAKE_CLAUDE_OUTPUT") or f"Summary {len(open(log).read().splitlines())}.")
