---
type: sprint
title: Harness tests pass whatever git's default branch
bead: yeeef-agents-9va.42
---

## Goal

> What should be true when this sprint ends, and why now?

`make test` passes on every machine the owner works from, including this Linux server, whatever git's `init.defaultBranch` is set to.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the 3 `test_review_wait_*` tests in `test_pm.py`, which fail on the server (365 passed, 3 failed) because their helper `review_with_origin` creates a bare origin with `git init --bare`. With `init.defaultBranch` unset, the origin's HEAD is `master`, the clone has an unborn `master`, and `git push origin HEAD:main` is rejected as non-fast-forward. Also any other test or harness code that assumes the default branch is `main`.

**Out:** setting `init.defaultBranch` in the owner's git config, since that would hide the assumption instead of removing it; the review-wait behaviour itself.

## Done when

> What evidence will show the goal is met?

- `make test` passes on the server with `init.defaultBranch` unset, and with it set to `master`.
- No test or harness code relies on git's default branch name; a grep for `git init` / `clone` without an explicit branch finds none that depend on it.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Cause found 2026-10-06: on the server (git 2.43, init.defaultBranch unset)
  the 3 test_review_wait_* tests fail at review_with_origin's 'git push -q
  origin HEAD:main' from a clone whose unborn default branch is master, a
  non-fast-forward push; they fail identically on main without PR #39.

- Fixing the origin's branch exposed a second server-only failure: the no-gh
  test hid gh with PATH=<tools>:/usr/bin:/bin, but apt installs gh at
  /usr/bin/gh (Homebrew's is outside). With the PATH limited to linked tools
  and the origin made with -b main, make test gives 374 passed, 7 skipped
  under init.defaultBranch unset, master and main; reverting only the -b main
  line gives 3 failed under master, 3 passed under main.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: `make test` passes on the server whatever git's default branch is.

- PR #41 (merged as 8f5c618): the review-wait tests' bare origin is made on `main`, and the no-gh test no longer relies on `gh` living outside /usr/bin.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **`make test` passes on the server with `init.defaultBranch` unset and set to `master`: met.** 374 passed, 7 skipped for unset, `master` and `main`; with only the `-b main` line reverted, `-k review_wait` gives 3 failed under `master`, 3 passed under `main`, so the setting reached the tests. Not run on the Mac.
- **No test or harness code relies on git's default branch name: met.** A grep for `init`/`clone` in `skills/project-management/harness` finds no harness code that runs either; the test bare origins are now made with `-b main` or cloned with `-b records`, and non-bare inits all pass `-b`.
