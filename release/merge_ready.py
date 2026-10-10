#!/usr/bin/env python3
"""Refuse to merge a pull request whose CI did not test what the merge lands (AGENTS.md, Tests). Standard library only.

  merge_ready.py N    exit 0 when PR N is open, its head contains its base branch as the remote has it now, and every
                      check on that head passed; print the merge command pinned to that head. Else exit 1, naming why.

Each PR's CI runs on its head merged with its base as the base was then. Two PRs that pass alone can break main
together; a head that contains the base, with every check passed, is the merged result tested.
"""

from __future__ import annotations

import json
import subprocess
import sys

PASSED = {"SUCCESS", "SKIPPED", "NEUTRAL"}


def run(*args: str) -> str:
    r = subprocess.run(args, capture_output=True, text=True)
    if r.returncode:
        raise SystemExit(f"merge-ready: {' '.join(args)} failed: {r.stderr.strip()}")
    return r.stdout


def fetched(ref: str) -> str:
    run("git", "fetch", "-q", "origin", ref)
    return run("git", "rev-parse", "FETCH_HEAD").strip()


def failed_checks(rollup: list[dict]) -> list[str]:
    """Each check on the head that has not passed, as one line; a head with no check has not passed either."""
    if not rollup:
        return ["no check ran on its head"]
    out = []
    for c in rollup:
        if c.get("__typename") == "StatusContext":
            if c.get("state") != "SUCCESS":
                out.append(f"status {c.get('context')!r} is {c.get('state')}")
        elif c.get("status") != "COMPLETED":
            out.append(f"check {c.get('workflowName')} / {c.get('name')} is {c.get('status')}")
        elif c.get("conclusion") not in PASSED:
            out.append(f"check {c.get('workflowName')} / {c.get('name')} concluded {c.get('conclusion')}")
    return out


def main(argv: list[str]) -> int:
    if len(argv) != 1 or not argv[0].isdigit():
        print("usage: merge_ready.py <PR number>", file=sys.stderr)
        return 2
    n = argv[0]
    pr = json.loads(run("gh", "pr", "view", n, "--json", "state,headRefOid,baseRefName,statusCheckRollup"))
    if pr["state"] != "OPEN":
        print(f"PR #{n} is not ready to merge: it is {pr['state']}", file=sys.stderr)
        return 1
    head, base = pr["headRefOid"], pr["baseRefName"]
    base_sha = fetched(f"refs/heads/{base}")
    why = []
    if fetched(f"refs/pull/{n}/head") != head:
        why.append("its head moved on the remote while this ran; run it again")
    elif subprocess.run(["git", "merge-base", "--is-ancestor", base_sha, head]).returncode:
        why.append(f"its head {head[:7]} does not contain origin/{base} {base_sha[:7]}, so its CI did not test what "
                   f"the merge lands: rebase it onto origin/{base} (or merge origin/{base} into it), push, and wait "
                   "for its CI")
    why += failed_checks(pr["statusCheckRollup"])
    if why:
        print(f"PR #{n} is not ready to merge:\n" + "\n".join(f"- {w}" for w in why), file=sys.stderr)
        return 1
    print(f"PR #{n} is ready: its head {head[:7]} contains origin/{base} {base_sha[:7]}, and its "
          f"{len(pr['statusCheckRollup'])} checks passed. Merge that head and only it:\n"
          f"  gh pr merge {n} --squash --match-head-commit {head}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
