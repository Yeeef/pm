"""Agents change code only in a worktree of their own, never in the main checkout, so sessions never mix edits,
branches or stashes. The main checkout is the worktree whose git dir is the clone's common git dir; the records store
and every `git worktree add` worktree are linked worktrees, so records writes pass. `pm task claim`, the git
pre-commit and post-checkout hooks and the Claude Code edit hook check it. PM_ALLOW_MAIN_CHECKOUT=1 lets the owner,
or a session the owner sends there on purpose, act in the main checkout. Standard library only: the hooks import it."""

from __future__ import annotations

import os
import subprocess
from pathlib import Path

ALLOW = "PM_ALLOW_MAIN_CHECKOUT"
DIR = ".claude/worktrees"  # where the how-to puts a worktree; Claude Code's EnterWorktree uses it too
AGENT_ENVS = ("CLAUDE_CODE_SESSION_ID", "CODEX_THREAD_ID")  # what each runtime exports to the commands it runs


def allowed() -> bool:
    return os.environ.get(ALLOW) == "1"


def agent() -> bool:
    """Whether an agent session runs this: the owner's own terminal sets neither runtime's session id."""
    return any(os.environ.get(k) for k in AGENT_ENVS)


def git(cwd: str | Path | None, *args: str) -> str:
    return subprocess.run(["git", *args], cwd=cwd, capture_output=True, text=True, timeout=5, check=True).stdout


def main_checkout(cwd: str | Path | None) -> Path | None:
    """The main checkout's root when `cwd` is in it, else None (a linked worktree, the records store among them)."""
    git_dir, common, top = git(cwd, "rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir",
                               "--show-toplevel").split("\n")[:3]
    return Path(top) if Path(git_dir).resolve() == Path(common).resolve() else None


def in_main_tree(cwd: str | Path | None, path: str | Path) -> Path | None:
    """The main checkout's root when `path` (relative to `cwd`) lies in its tree outside every linked worktree of the
    clone `cwd` is in, else None. `records/` resolves into the store, a linked worktree, so it passes."""
    p = (Path(cwd or ".") / path).resolve()
    trees = [Path(l[len("worktree "):]).resolve()
             for l in git(cwd, "worktree", "list", "--porcelain").splitlines() if l.startswith("worktree ")]
    main, linked = trees[0], trees[1:]
    if not p.is_relative_to(main) or any(p.is_relative_to(t) for t in linked):
        return None
    return main


def howto(main: Path, remote: str, branch: str) -> str:
    """How to get out of the main checkout, as every refusal says it."""
    return (f"{main} is the main checkout; agents change code only in a worktree of their own. Make one and work "
            f"there: `git -C {main} fetch {remote} {branch} && git -C {main} worktree add -b <branch> "
            f"{DIR}/<name> {remote}/{branch}`, then `cd {main}/{DIR}/<name>` (in Claude Code, the EnterWorktree tool "
            f"does the same); pm's post-checkout hook and session start link its records/ (else run `pm init` "
            f"there). A subagent works in its parent's worktree. Only when the owner asks for work in the main "
            f"checkout, set {ALLOW}=1.")
