#!/usr/bin/env python3
"""Whether a pull request's changes can affect pm's Go jobs (pm-go.yml, pm-lint.yml's Go linters), so a PR that changes
only docs or the harness's Python skips their work while each of their checks still reports.

    go_changes.py    writes go=true or go=false to $GITHUB_OUTPUT and prints why

On any event but a pull request it is true: a push to main runs every job. On a pull request the checkout is the PR's
merge commit fetched two deep, and its changes are the diff against its first parent, the base. It is false only when
there are changes and each one matches NO_GO and no Go file reads it: a Go file reaches a repo file by a path up from
its package (`../../tests/fake_sched.py`, or filepath.Join("..", "..", "go.mod")), or, in the module root's package,
by its plain path. README.md, though listed, never skips them today: assets_test.go, in the root package, names
"README.md" (changelog.d's), which reads as the root's. A checkout that is not a merge commit with both parents (one
fetched too shallow, or a pull_request event checked out at its head), and any git failure, fail the step: nothing is
guessed.
"""

from __future__ import annotations

import fnmatch
import os
import subprocess
import sys
from pathlib import Path, PurePosixPath

# The paths no Go build, vet, lint or test reads: the docs no file embeds (prime.md and CHANGELOG.md are embedded),
# the site's static page, and the harness's and the release tools' Python. README.md stays read by Go while a root
# package file names "README.md" (assets_test.go does, for changelog.d's).
NO_GO = ("README.md", "AGENTS.md", "CLAUDE.md", "website/*", "tests/*.py", "release/*.py")


def git(*args: str) -> str:
    return subprocess.run(["git", *args], capture_output=True, text=True, check=True).stdout


def read_by_go(path: str, go_files: dict[str, str]) -> bool:
    """Whether a Go file names path as one it reads (go_files: each tracked .go file's repo path and text)."""
    parts = PurePosixPath(path).parts
    up = ("../" + path, '"..", ' + ", ".join(f'"{p}"' for p in parts))
    return any(any(u in text for u in up) or ("/" not in name and f'"{path}' in text) for name, text in go_files.items())


def go_affected(changed: list[str], go_files: dict[str, str]) -> tuple[bool, str]:
    """Whether changed (repo paths) can affect the Go jobs, and why."""
    if not changed:
        return True, "no changed file found: every job runs"
    for path in changed:
        if not any(fnmatch.fnmatchcase(path, p) for p in NO_GO):
            return True, f"{path} can affect the Go jobs"
        if read_by_go(path, go_files):
            return True, f"{path} is read by a Go file"
    return False, f"none of the {len(changed)} changed files can affect the Go jobs: {' '.join(changed)}"


def main() -> int:
    if os.environ.get("GITHUB_EVENT_NAME") != "pull_request":
        go, why = True, f"a {os.environ.get('GITHUB_EVENT_NAME')} event runs every job"
    else:
        if subprocess.run(["git", "rev-parse", "--verify", "-q", "HEAD^2"], capture_output=True, check=False).returncode:
            print("go_changes.py: HEAD is not a pull request's merge commit with its two parents fetched (checkout "
                  "with fetch-depth: 2 at the merge ref); refusing to guess the PR's changes", file=sys.stderr)
            return 1
        changed = git("diff", "--name-only", "--no-renames", "HEAD^1", "HEAD").split()
        go_files = {f: Path(f).read_text(encoding="utf-8") for f in git("ls-files", "*.go").split()}
        go, why = go_affected(changed, go_files)
    print(f"go={'true' if go else 'false'}: {why}")
    with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as out:
        out.write(f"go={'true' if go else 'false'}\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
