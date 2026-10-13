"""The AGENTS.md evaluation's labelled cases: the checkout's AGENTS.md, which passes every criterion, and copies of the
checkout each with one planted defect, which must fail on the criterion the defect breaks.

A case is a set of edits to the checkout's files, each a function of the file's text that fails hard when the text it
looks for is gone, so a later edit of AGENTS.md that a case no longer fits shows up as an error, never as a case that
tests nothing. A full-mode case edits AGENTS.md; a diff-mode case edits other files, and the evaluation reviews the
diff. Each label comes from the rubric (rubric.md), never from what the evaluation answers.

A planted case's run gives "fail" when its criterion fails with a finding that names the planted text (`marker`), so
a fail for another reason does not count; the other criteria are not run. A clean case's run gives "pass" when every
criterion it runs passes.
"""

from __future__ import annotations

import os
import re
import shutil
import subprocess
from collections.abc import Callable
from dataclasses import dataclass, field
from pathlib import Path

from agents_md import evaluate

Edit = Callable[[str, Path], str]  # a file's text and the checkout it is in, to the edited text


@dataclass(frozen=True)
class Case:
    name: str
    mode: str  # "full" or "diff"
    criterion: str | None  # the criterion the defect breaks; None: a clean case, every criterion passes
    edits: dict[str, Edit] = field(default_factory=dict)  # by path in the checkout
    marker: str = ""  # what a finding of the failing criterion names: the planted text, or the rule removed

    @property
    def expect(self) -> str:
        return "pass" if self.criterion is None else "fail"


def after_heading(pattern: str, text: str | Edit) -> Edit:
    """Insert `text` (or what it gives for the file) as a paragraph under the first `## ` heading matching `pattern`."""
    def edit(src: str, root: Path) -> str:
        para = text(src, root) if callable(text) else text
        m = re.search(rf"^## .*{pattern}.*\n", src, flags=re.M | re.I)
        if not m:
            raise ValueError(f"no ## heading matches {pattern!r}")
        return f"{src[:m.end()]}\n{para}\n{src[m.end():]}"
    return edit


def drop_lines(pattern: str) -> Edit:
    """Remove every line that matches `pattern`."""
    def edit(src: str, _root: Path) -> str:
        lines = src.splitlines(keepends=True)
        kept = [line for line in lines if not re.search(pattern, line)]
        if len(kept) == len(lines):
            raise ValueError(f"no line matches {pattern!r}")
        return "".join(kept)
    return edit


def replace(old: str, new: str) -> Edit:
    def edit(src: str, _root: Path) -> str:
        if old not in src:
            raise ValueError(f"{old!r} is not in the file")
        return src.replace(old, new)
    return edit


def prime_bodies(_src: str, root: Path) -> str:
    """prime.md's Bodies paragraph, as prime.md has it now."""
    m = re.search(r"^\*\*Bodies\.\*\*.*$", (root / "prime.md").read_text(), flags=re.M)
    if not m:
        raise ValueError("prime.md has no **Bodies.** paragraph")
    return m.group(0)


TOUR = """`internal/work` file by file: `item.go` holds the Item type, `store.go` the Store interface, `schema.go` the
schema, `check.go` the invariant checks on every write and load, `ids.go` the ids, `ready.go` ready and blocked,
`importbd.go` the bd import, `export.go` `pm export` and its import, `merge.go` the merge rules, `sync.go` the sync
through the git remote, `move.go` a sprint move, `host.go` the host, `dolt.go` the client, `lock.go` the write lock,
and `service.go` the operations that run in the service."""

DETAIL = """### The work store's host

The host is the Dolt engine held open by `pm service run` and served on `<main>/.pm/run/work.sock`. It registers the
stored procedures `pm_version()`, which answers the version handshake `work.Dial` makes before any query;
`pm_sync()`, which fetches `refs/pm/work`, merges it with the rules in `merge.go` and pushes the result; `pm_create(?)`,
which takes the next child id with a compare-and-swap on the scratch branch `pm-cas`; `pm_move_sprint(?)`, which runs
a sprint move under that same compare-and-swap; `pm_setup()`; and `pm_lock()` and `pm_unlock()`, the store's fair write
lock from `lock.go`, which every write a command makes takes first. A client holds one connection to the socket for
the command's life and closes it at exit; the service's own loops (sync, gc, the change mark) reach the host through
the same socket, as any command does. `access_test.go` holds that only `host.go` loads an engine and only
`pm service run` starts a host."""

CASES = {c.name: c for c in [
    Case("clean", "full", None),
    Case("accurate_unknown_target", "full", "accurate",
         {"AGENTS.md": after_heading("Test", "Run `make test-smoke` before `make test`: it builds pm and runs the ten "
                                             "light tests of the command parser, in under ten seconds.")},
         marker="make test-smoke"),
    Case("accurate_wrong_behaviour", "full", "accurate",
         {"AGENTS.md": after_heading("Test", "`make lint` runs `make go-build` first, so a lint run also rebuilds "
                                             "`.go/pm`.")},
         marker="runs make go-build first"),
    Case("non_inferable_tour", "full", "non_inferable", {"AGENTS.md": after_heading("", TOUR)},
         marker="file by file"),
    Case("map_detail", "full", "map", {"AGENTS.md": after_heading("", DETAIL)}, marker="pm_version()"),
    Case("no_copies_bodies", "full", "no_copies", {"AGENTS.md": after_heading("", prime_bodies)},
         marker="pm reads stdin only for"),
    Case("actionable_dont", "full", "actionable", {"AGENTS.md": after_heading("", "Don't break the hooks.")},
         marker="Don't break the hooks"),
    Case("no_history_dated", "full", "no_history",
         {"AGENTS.md": after_heading("", "The judge's live eval on 2026-10-12, 63 cases: Claude 189/189, median 2.0 s, "
                                         "p90 3.6 s; Codex 189/189, median 5.5 s.")},
         marker="189/189"),
    Case("consistent_merge", "full", "consistent",
         {"AGENTS.md": after_heading("", "Merge a PR with `gh pr merge <n> --squash` directly, once its checks pass.")},
         marker="directly, once its checks pass"),
    Case("complete_merge_ready", "full", "complete", {"AGENTS.md": drop_lines(r"merge-ready|merge_ready")},
         marker="merge-ready"),
    Case("complete_pm_launched", "full", "complete", {"AGENTS.md": drop_lines(r"PM_LAUNCHED")},
         marker="PM_LAUNCHED"),
    Case("diff_renamed_target", "diff", "accurate", {"Makefile": replace("merge-ready", "merge-check")},
         marker="merge-ready"),
    Case("diff_neutral", "diff", None,
         {".github/scripts/go_cache_trim.py": replace("Trim a restored Go build cache to what",
                                                      "Trim a restored Go build cache down to what")}),
]}


def edited(case: Case, root: Path) -> dict[str, str]:
    """The case's files as its edits leave them, from the checkout at `root`; each edit must change its file."""
    out = {}
    for path, edit in case.edits.items():
        src = (root / path).read_text()
        out[path] = edit(src, root)
        if out[path] == src:
            raise ValueError(f"{case.name}: the edit of {path} changes nothing")
    return out


def git(root: Path, *args: str) -> str:
    return subprocess.run(["git", "-C", str(root), "-c", "user.name=t", "-c", "user.email=t@example.com", *args],
                          capture_output=True, text=True, check=True).stdout


def materialize(case: Case, src: Path, dest: Path) -> str:
    """A git repo at `dest` holding the checkout's tracked files as its working tree has them, committed as the base,
    then the case's edits committed on top; the base commit's sha."""
    files = git(src, "ls-files", "-z").split("\0")
    for name in filter(None, files):
        s, d = src / name, dest / name
        if not (s.exists() or s.is_symlink()):
            continue  # deleted in the working tree
        d.parent.mkdir(parents=True, exist_ok=True)
        if s.is_symlink():
            os.symlink(os.readlink(s), d)
        else:
            shutil.copy2(s, d)
    git(dest, "init", "-q", "-b", "main")
    git(dest, "add", "-A")
    git(dest, "commit", "-q", "-m", "base")
    base = git(dest, "rev-parse", "HEAD").strip()
    for path, text in edited(case, dest).items():
        (dest / path).write_text(text)
    if case.edits:
        git(dest, "commit", "-q", "-am", case.name)
    return base


def calls(case: Case, root: Path, base: str) -> list[evaluate.Call]:
    """The evaluation's calls on a materialized case: its criterion alone for a planted full-mode case."""
    agents_md = (root / "AGENTS.md").read_text()
    if case.mode == "diff":
        return evaluate.diff_calls(agents_md, evaluate.pr_diff(root, base)[0])
    return evaluate.full_calls(agents_md, root, (case.criterion,) if case.criterion else evaluate.CRITERIA)


def planted(root: Path, base: str) -> str:
    """The lines the case's edits added to AGENTS.md."""
    diff = git(root, "diff", "-U0", base, "HEAD", "--", "AGENTS.md")
    return "\n".join(line[1:] for line in diff.splitlines() if line.startswith("+") and not line.startswith("+++"))


def outcome(case: Case, results: list[evaluate.Result], added: str = "") -> str:
    """One run's outcome: "error: …" when a call gave no answer; for a planted case "fail" when its criterion failed on
    a finding that names the marker or quotes the lines the case added (`added`), "fail elsewhere: …" when it failed
    on others alone, else "pass"; for a clean case "pass", or "fail: <the criteria that failed>"."""
    if errors := [r.error for r in results if r.error]:
        return f"error: {errors[0]}"
    verdict = evaluate.verdicts(results)
    if case.criterion is None:
        failed = sorted(c for c, v in verdict.items() if v != "pass")
        return f"fail: {','.join(failed)}" if failed else "pass"
    if verdict[case.criterion] == "pass":
        return "pass"
    findings = [f for r in results if r.call.criterion == case.criterion for f in r.findings]
    mark = evaluate.normal(case.marker)
    if any(mark in evaluate.normal(f["quote"] + " " + f["problem"])
           or (evaluate.normal(f["quote"]) and evaluate.normal(f["quote"]) in evaluate.normal(added)) for f in findings):
        return "fail"
    return "fail elsewhere: " + "; ".join(f["quote"][:80] or f["problem"][:80] for f in findings)
