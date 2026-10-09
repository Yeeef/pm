#!/usr/bin/env python3
"""A stand-in for `gh` in tests: `gh pr view <url> --json …` prints the PR's entry in $FAKE_GH_STATE (a JSON object
keyed by URL) and fails as gh does for a PR it does not know; `gh auth token` prints $FAKE_GH_TOKEN, and without it
fails as gh does when not logged in."""

import json
import os
import sys

args = sys.argv[1:]
prs = json.load(open(os.environ["FAKE_GH_STATE"]))
if args == ["auth", "token"]:  # $FAKE_GH_TOKEN, or not logged in
    if not os.environ.get("FAKE_GH_TOKEN"):
        print("no oauth token found for github.com", file=sys.stderr)
        sys.exit(1)
    print(os.environ["FAKE_GH_TOKEN"])
elif args[:2] == ["pr", "view"] and len(args) > 2:
    if args[2] not in prs:
        print(f"GraphQL: Could not resolve to a PullRequest ({args[2]})", file=sys.stderr)
        sys.exit(1)
    print(json.dumps(prs[args[2]]))
else:
    print(f"fake gh: unsupported {args}", file=sys.stderr)
    sys.exit(1)
