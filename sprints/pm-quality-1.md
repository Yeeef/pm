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

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
