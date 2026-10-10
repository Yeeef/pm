---
type: sprint
title: "Port option C to Go pm: drop main's records/ copy, keep the link"
bead: pm-d2k5.1
---

## Goal

> What should be true when this sprint ends, and why now?

Go pm in Yeeef/pm builds the owner's option C: main keeps no `records/` copy, nothing exists only for that copy, and each worktree keeps its `records/` link. The owner chose option C in [pm-harness sprint 56](../sprints/pm-harness-56.md), which listed every reader of the link and of main's copy.

Why now: option C was built for Python pm in yeeef-agents PR #76. That PR's review was dismissed when Go pm replaced Python pm, so the build never shipped. Go pm's `pm init` still writes `pm-records-copy.yml` and `pm-records-guard.yml` (internal/install pieces.go) and sets up the sparse checkout (clone.go).

Moved from pm-harness sprint 56 on 2026-10-10.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Port option C to Go pm in internal/install (pieces.go, clone.go): no sparse checkout of `records/`, and no copy or guard workflow.
- Remove main's `records/` copy, the copy Action, the PR guard, the pre-commit guard and the sparse checkout; keep the link.
- `pm doctor` reports these pieces in an installed repo, and `pm upgrade` and `pm init` remove them.
- Design pages updated to the new layout.

**Out:**
- The location of the store itself.
- The decision itself: pm-harness sprint 56 holds the readers doc, the options and the owner's answer.

## Done when

> What evidence will show the goal is met?

- Once the PR is on Yeeef/pm's main, `git ls-tree origin/main records` prints nothing and `.github/workflows` holds no `pm-records-copy.yml` or `pm-records-guard.yml`.
- In a new worktree, `pm init` makes the link and no sparse checkout: `git config core.sparseCheckout` is unset.
- In a repo installed by an older pm, `pm doctor` reports the copy pieces and `pm upgrade` removes them.
- The pm test suite passes in CI.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Rebasing the branch that deletes main's records/ copy onto a main the
  copy Action had moved (11 records files changed or added) stopped on 9
  modify/delete conflicts, and git wrote those 9 files into a real records/
  directory in place of the worktree's git-ignored link (git treats an ignored
  path as expendable). The store was untouched (status clean, its own HEAD);
  resolved by git rm --cached --sparse of every records/ path, removing the
  directory and relinking. The merge of this PR meets the same conflicts while
  the copy Action runs, so the branch needs a rebase right before the merge.

- git sparse-checkout disable leaves core.sparseCheckout=false and the
  patterns file, and git worktree add copied that false into a new worktree
  (live check, git 2.43.0); Unsparse now also unsets the worktree's sparse
  keys and deletes info/sparse-checkout, after which core.sparseCheckout is
  unset in the main checkout and a new worktree.

- Live check (scratch origin and clone under /tmp, fake
  HOME/CODEX_HOME/supervisor): a repo installed by the pm 0.4.0 release with
  main's copy made by the copy Action's own git subtree add; this build's pm
  doctor named 3 retired files and the tracked copy, pm upgrade removed them
  and its printed commit untracked records/ (12 files changed), after merge
  and push git ls-tree origin/main records was empty with no workflows, pm
  init turned the sparse checkout off, a new worktree got the link with
  core.sparseCheckout unset, pm doctor was clean in both, and an old branch
  was fixed by merging main.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: Go pm builds option C in Yeeef/pm PR #20, merged as 6a0bb49: it no longer writes main's records/ copy or the pieces that served it, and each worktree keeps its records/ link.

- `pm init` writes no copy workflow, no PR guard and no pre-commit section, and sets no sparse checkout. It turns an earlier pm's sparse checkout off, leaving its config and file unset, once the worktree tracks no `records/`.
- The three tracked pieces are retired pieces. `pm doctor` names them, and `pm upgrade` and `pm uninstall` remove them. `pm doctor` also names a branch that still tracks `records/`, and `pm upgrade` prints the commit that untracks it. `pm init` and `pm where` tell a worktree on such a branch to merge main.
- `pm hook git-pre-commit` is a no-op, so an old pre-commit section in the main checkout does not block a worktree on the new pin.
- In this repo, main's `records/` is untracked from the index only (the store is untouched), and both workflows and `.pm/hooks/pre-commit` are deleted.
- Docs updated: `prime.md`, CLAUDE.md's layout, the `--help` texts, the CHANGELOG (a breaking change with an upgrade guide), and the records-store and pm-cli design pages.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- `git ls-tree origin/main records` prints nothing and `.github/workflows` has neither workflow: met: after the merge (6a0bb49), `git ls-tree origin/main records` prints nothing (0 lines) and `.github/workflows` holds pm-changelog, pm-go, pm-release-notes, pm-release and pm-tests. The main checkout's `records` link survived the pull (126 sprint records through it).
- In a new worktree, `pm init` makes the link and `core.sparseCheckout` is unset: met for this build. The live check shows the link and `core.sparseCheckout` unset in the main checkout and in a new worktree. `tests/test_init.py` checks the same for a fresh install. This repo pins 0.4.0, and that release's `pm init` still sets the sparse checkout. So this holds here only once a release with this change is pinned, which is an ordinary PR after the release.
- In a repo installed by an older pm, `pm doctor` reports the copy pieces and `pm upgrade` removes them: met.
  - Harness: `test_upgrade_retires_the_main_branchs_records_copy`, plus the retired workflow in `test_doctor_reports_each_changed_repo_piece_and_upgrade_restores_it` and the sparse pattern in `test_doctor_reports_each_changed_clone_setup`.
  - Go: `TestRetiredPiecesAreReportedAndRemoved` and the golden table.
  - Live check against a repo installed by the real 0.4.0 release (Findings).
- The pm test suite passes in CI: met, all five checks on PR #20 green (macOS after one rerun of the known Dolt UpdateGCGen flake, pm-quality sprint 3). Locally, `make test` gives 112 passed, 40 skipped. `test_hooks.py`, `test_init.py` and `test_lifecycle.py` give 27 passed. `go test` passes for install, cli, hooks and site.
