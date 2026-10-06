#!/usr/bin/env python3
"""A stand-in for `claude -p` in tests: appends its arguments and stdin to $FAKE_CLAUDE_LOG as one JSON line, then
answers "Summary <n>." (n counting its calls), or, when $FAKE_CLAUDE_FAIL is set, fails with that text on stderr."""

import json
import os
import sys

log = os.environ["FAKE_CLAUDE_LOG"]
with open(log, "a") as f:
    f.write(json.dumps({"args": sys.argv[1:], "stdin": sys.stdin.read()}) + "\n")
if os.environ.get("FAKE_CLAUDE_FAIL"):
    print(os.environ["FAKE_CLAUDE_FAIL"], file=sys.stderr)
    sys.exit(1)
print(f"Summary {len(open(log).read().splitlines())}.")
