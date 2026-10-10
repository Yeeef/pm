---
type: sprint
title: Fix the flaky pm tests
bead: yeeef-agents-9va.106
---

## Goal

> What should be true when this sprint ends, and why now?

The pm tests stop failing at random on CI, so a red pm job means a real fault. Two known flakes: the test fixture's `git worktree add` exits 128 on Linux under `-n auto`, and `test_a_push_that_failed_is_retried_by_the_sweep` fails intermittently.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Show git's stderr in the fixture's git error, then fix the root cause of the exit 128.
- Find and fix the root cause of the push-sweep test flake.

**Out:**
- Other test speed or layout changes.

## Done when

> What evidence will show the goal is met?

- The pm go and pm tests jobs pass on several consecutive full-suite CI runs with `-n auto`, with neither flake.

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

- test_a_push_that_failed_is_retried_by_the_sweep no longer exists: commit
  888c7f7 pruned it. Restored locally, it failed 9/12 and 7/12 because it
  checked the sweep's log line after seeing the bd mark, which pm writes
  before the line; waiting for the line made it pass 42/42. The restore was
  dropped so the prune stands. The fake bd also wrote its state in place
  (JSONDecodeError under polling); now atomic (PR #85).

- The git worktree add exit 128 hits only the Linux pm go job (test_go_parity,
  -n auto), always on xdist worker gw1, in 4 runs since main c03d436a
  (go-port-skeleton merge). 0 hits in 30+ local macOS runs, including with git
  2.55.0 built from source. Repo.git now prints git's stderr to capture the
  cause on CI.

- Root cause of the exit 128, captured on CI run 37877661908 (rerun 3):
  "fatal: could not open '.git/worktrees/records/locked' for writing: No such
  file or directory". git 2.55's commit starts git maintenance run --auto
  --detach; its worktree-prune deletes a worktree admin dir with no gitdir
  file yet, which is the state git worktree add is in right after the
  fixture's commit. Fix: maintenance.auto=false in the repo fixture. GIT_TRACE
  on git 2.55: 3 maintenance invocations per commit before, 0 after.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: both flakes are gone; the exit 128 had a root cause in git 2.55's background maintenance, and the sweep-test flake was already gone with its test.

Merged as 4f51d6d (PR #85).

- The repo fixture turns off git auto maintenance (`maintenance.auto=false`), so no detached worktree-prune races `git worktree add`.
- The fixture's git errors print git's stderr.
- The fake bd writes its state atomically, so a test polling it never reads an empty file.
- The sweep test stays removed, as commit 888c7f7 decided.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- The pm go and pm tests jobs pass on several consecutive full-suite CI runs with `-n auto`, with neither flake: met. On PR #85 the pm go workflow (Linux and macOS) passed consecutive reruns after the fix (4 of 4 at writing, up to 8 running), where before the fix it failed on rerun 3; pm tests (light and integration) passed. The sweep test no longer exists, so it cannot flake.
