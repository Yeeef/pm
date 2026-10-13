#!/usr/bin/env python3
"""Refuse to merge a pull request whose CI did not test what the merge lands (AGENTS.md, Tests). Standard library only.

  merge_ready.py N    exit 0 when PR N is open, its head contains its base branch as the remote has it now, every
                      check on that head passed (a check that ran twice, by its latest run), and every workflow that
                      runs on each PR (.github/workflows, a pull_request trigger without paths) has a check there;
                      print the merge command pinned to that head. Else exit 1, naming why.

Each PR's CI runs on its head merged with its base as the base was then. Two PRs that pass alone can break main
together; a head that contains the base, with every check passed, is the merged result tested.

A head may lack the release workflow's own commits on the base (Releasing pm): each one, by github-actions[bot],
writes a release's section into CHANGELOG.md and deletes the entry files it released, which ship nothing and run no
test but the changelog's format, so a PR need not rebase and re-run its CI for them. The PR must touch neither
CHANGELOG.md nor a deleted entry file.
"""

from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PASSED = {"SUCCESS", "SKIPPED", "NEUTRAL"}
BOT = "41898282+github-actions[bot]@users.noreply.github.com"  # the author of the release workflow's commits


def run(*args: str) -> str:
    r = subprocess.run(args, capture_output=True, text=True, check=False)
    if r.returncode:
        raise SystemExit(f"merge-ready: {' '.join(args)} failed: {r.stderr.strip()}")
    return r.stdout


def fetched(ref: str) -> str:
    run("git", "fetch", "-q", "origin", ref)
    return run("git", "rev-parse", "FETCH_HEAD").strip()


def pr_workflows(root: Path) -> set[str]:
    """The names of the workflows that run on every pull request: a pull_request trigger with no paths filter."""
    names = set()
    for f in sorted((root / ".github" / "workflows").glob("*.yml")):
        lines = f.read_text().splitlines()
        name = next((line[6:].strip() for line in lines if line.startswith("name: ")), None)
        if "  pull_request:" not in lines:
            continue
        i = lines.index("  pull_request:") + 1
        block = []
        while i < len(lines) and (lines[i].startswith("    ") or not lines[i].strip()):
            block.append(lines[i].strip())
            i += 1
        if name and not any(b.startswith(("paths:", "paths-ignore:")) for b in block):
            names.add(name)
    return names


def is_release_commit(author: str, changes: list[str]) -> bool:
    """A release commit: by the release workflow, changing CHANGELOG.md and deleting entry files, nothing else
    (changes: git diff-tree --name-status lines)."""
    return author == BOT and bool(changes) and all(
        c == "M\tCHANGELOG.md" or (c.startswith("D\tchangelog.d/") and c.endswith(".md")) for c in changes)


def lacks_only_release_commits(base_sha: str, head: str) -> bool:
    """Whether every commit of the base that head lacks is a release commit, and head's own changes touch none of
    their files."""
    touched: set[str] = set()
    for c in run("git", "rev-list", base_sha, f"^{head}").split():
        changes = run("git", "diff-tree", "--no-commit-id", "--name-status", "-r", "--no-renames", c).splitlines()
        if not is_release_commit(run("git", "log", "-1", "--format=%ae", c).strip(), changes):
            return False
        touched |= {line.split("\t", 1)[1] for line in changes}
    base = run("git", "merge-base", base_sha, head).strip()
    return not touched & set(run("git", "diff", "--name-only", "--no-renames", f"{base}..{head}").split())


def failed_checks(rollup: list[dict], required: set[str]) -> list[str]:
    """Each check on the head that has not passed, as one line, judging a check that ran more than once on the head
    (a re-run, or a run cancelled by a later one) by its latest run; each workflow in required with no check on the
    head, which has not run yet."""
    out, runs = [], {}
    for c in rollup:
        if c.get("__typename") == "StatusContext":
            if c.get("state") != "SUCCESS":
                out.append(f"status {c.get('context')!r} is {c.get('state')}")
        else:
            runs.setdefault((c.get("workflowName"), c.get("name")), []).append(c)
    for (workflow, name), cs in runs.items():
        if pending := [c for c in cs if c.get("status") != "COMPLETED"]:
            out.append(f"check {workflow} / {name} is {pending[0].get('status')}")
        else:
            latest = max(cs, key=lambda c: (c.get("startedAt") or "", c.get("completedAt") or ""))
            if latest.get("conclusion") not in PASSED:
                out.append(f"check {workflow} / {name} concluded {latest.get('conclusion')}")
    ran = {workflow for workflow, _ in runs}
    out += [f"no check of workflow {w!r}, which runs on every PR, is on its head yet" for w in sorted(required - ran)]
    if not rollup and not required:
        out.append("no check ran on its head")
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
    elif subprocess.run(["git", "merge-base", "--is-ancestor", base_sha, head], check=False).returncode and \
            not lacks_only_release_commits(base_sha, head):
        why.append(f"its head {head[:7]} does not contain origin/{base} {base_sha[:7]}, so its CI did not test what "
                   f"the merge lands: rebase it onto origin/{base} (or merge origin/{base} into it), push, and wait "
                   "for its CI")
    why += failed_checks(pr["statusCheckRollup"], pr_workflows(ROOT))
    if why:
        print(f"PR #{n} is not ready to merge:\n" + "\n".join(f"- {w}" for w in why), file=sys.stderr)
        return 1
    contains = "contains" if not subprocess.run(["git", "merge-base", "--is-ancestor", base_sha, head],
                                                 check=False).returncode else "lacks only release commits of"
    print(f"PR #{n} is ready: its head {head[:7]} {contains} origin/{base} {base_sha[:7]}, and every "
          "check on it passed. Merge that head and only it:\n"
          f"  gh pr merge {n} --squash --match-head-commit {head}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
