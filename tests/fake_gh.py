#!/usr/bin/env python3
"""A stand-in for `gh` in tests: `gh pr view <url> --json …` prints the PR's entry in $FAKE_GH_STATE (a JSON object
keyed by URL) and fails as gh does for a PR it does not know."""

import json
import os
import sys

args = sys.argv[1:]
prs = json.load(open(os.environ["FAKE_GH_STATE"]))
if args[:2] == ["pr", "view"] and len(args) > 2:
    if args[2] not in prs:
        print(f"GraphQL: Could not resolve to a PullRequest ({args[2]})", file=sys.stderr)
        sys.exit(1)
    print(json.dumps(prs[args[2]]))
else:
    print(f"fake gh: unsupported {args}", file=sys.stderr)
    sys.exit(1)
