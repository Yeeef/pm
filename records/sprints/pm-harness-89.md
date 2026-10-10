---
type: sprint
title: "Go port: work-store commands and sync (P7)"
bead: yeeef-agents-9va.98
---

## Goal

> What should be true when this sprint ends, and why now?

Agents can do everything they did with plain `bd` through Go pm, and two clones keep one work store in step through the git remote. These commands have no Python counterpart, so the work store design page, not parity, defines them.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- The commands with no Python counterpart: `task ready/edit/release`, `dep add/rm`, `comment add`, `need dismiss`, `reply add`, `export`, `sync`.
- Dolt sync through the git remote; the merge rules on conflict rows; the child-id compare-and-swap.

**Out:** the service's sync loop (service sprint); `pm init` attaching a clone to the remote store (install sprint).

## Done when

> What evidence will show the goal is met?

- `go test ./internal/work` passes tests derived from the work store page, not from the code: one case per row of the merge table, ids and minting, ready and blocked with cycle detection.
- A 2-clone sync test against a bare remote passes: concurrent edits on both clones merge per the table, and concurrent child mints get distinct ids.
- The sprint's PR is on main.

## Design pages

> Where is the detail?

- [Work store: pm's own replacement for Beads](../design/work-store.md): Storage, Ids, Ready and blocked, Commands
- [pm in Go](../design/pm-go.md): Port order and coexistence

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-08}
The work store's Dolt remote uses pm's own ref refs/pm/work on the repo's remote (DOLT_REMOTE add and DOLT_CLONE with --ref), and pm turns off Dolt's __dolt_remote_info__ info branch (DOLT_REMOTE_INFO_BRANCH set empty in the pm process).
Measured on the pinned Dolt (dolthub/dolt/go a6690826d767, driver v2.2.0): --ref is accepted for remotes of the git+ schemes, stored as git_ref in dolt_remotes params, and a push to a bare repo writes refs/pm/work and leaves refs/dolt/data alone, so per the page's default pm keeps its own ref and need not take bd's over at the cut-over. Dolt also force-pushes a visible branch __dolt_remote_info__ after every data push, and this repo's origin already has bd's; pm's push would overwrite it and cost one more push per sync, so pm disables it.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Dolt's remote ref is configurable (pinned dolthub/dolt/go a6690826d767):
  DOLT_REMOTE add and DOLT_CLONE take --ref, stored as git_ref in dolt_remotes
  params; a push to a bare repo wrote refs/pm/work and no refs/dolt/data. Dolt
  also force-pushes a visible branch __dolt_remote_info__ after each data push
  (this repo's origin has bd's); DOLT_REMOTE_INFO_BRANCH set empty turns it
  off, and TestSyncPushesUnderPmsOwnRefOnly checks the remote holds only
  refs/pm/work. The page's Open question 'Dolt remote ref' and its Remote row
  can now say so.

- Measured Dolt merge behaviour that shaped the sync: a conflicted items row
  keeps ours whole in the working set (theirs' non-conflicting cells are not
  applied), so pm merges every column three-way; since every write sets
  updated_at, any two concurrent edits of one item conflict as a row. Two
  clones adding a comment to one item at the same pos hit unique (item_id,
  pos) as a constraint violation, so schema 2 drops that index (comments order
  by pos, created_at, id). DOLT_MERGE --no-commit still fast-forwards outside
  the transaction, so a failed pull must reset to its pre-pull commit (found
  in review, reproduced by TestPullFastForwardThatFailsItsCheckResets failing
  without the reset).

- Dolt's git remote skips a fetch within 1 s of that remote's last read in the
  process (syncForRead TTL), and an engine close tears down every git remote
  cache in the process. Harmless for pm's one store per process (a real push
  timeout is 60 s), but the 2-clone tests keep a second clone open across the
  first's write (openA), and the outcome-unknown test removes the remote
  instead of the repo.

- Tests: go test ./internal/work 85 s locally (60 s before the rebase), of
  which 17 two-clone tests on a local bare repo take 2-10 s each;
  merge_test.go has 12 unit tests, one per items-field row of the merge table;
  the list rows (comments, labels, blocked_by) and close beats claim, add/add,
  cross-clone cycle, no-winner conflict and fast-forward rollback run through
  two clones. Concurrent child mints: A minting between B's pull and push
  gives A .3, B .4 after one rejected push; sprints get numbers 2 and 3; 3
  rejected pushes fail with nothing created. internal/cli: 11 tests, every
  refusal leaves the store equal. CI on 2ac8b55 green (pm go on macOS and
  Linux, pm tests light and integration). Local make test had 4-10
  stdin-timing failures in test_pm.py at load average 80 on 8 cores; they pass
  when run alone (19 passed) and in CI.

- Fresh-context review of PR #87: 3 correctness findings, all fixed in
  2ac8b55: a fast-forward pull that failed its checks kept the bad commit (now
  reset); a remote schema newer than pm's was merged and committed over (now
  refused); a timed-out push checked only the remote head, so a push another
  clone built on would be created twice (now: the commit in the remote's
  history). Also fixed: any non-rejection push error goes through the outcome
  check, the conflicts session variable is reset. Left open: overridden claims
  are printed by pm sync only and lost in a service sync, so pm show cannot
  warn as the merge table says; the commands take --text besides --text-file -
  (pm's convention) where the page says text comes on stdin; the clone
  (DOLT_CLONE) prints progress to stdout, for pm init's attach in the install
  sprint.

- Correction to the tests finding above: sync_test.go has 15 tests, 13 of them
  on two clones and a bare repo (not 17).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: Go pm has the work-store commands with no Python counterpart and syncs the work store through the git remote, with the merge rules and the child-id compare-and-swap.

Merged as 469930d (PR #87).

- `pm task ready/edit/release`, `dep add/rm`, `comment add`, `need dismiss`, `reply add`, `sync`.
- Sync under pm's own ref `refs/pm/work`; merge rules on `dolt_conflicts_items`; a pull that fails its checks rolls back to its pre-pull commit; a remote schema newer than pm's is refused.
- Child creates go through the compare-and-swap; the comments schema (version 2) lets two clones comment on one item.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Met:** `go test ./internal/work` passes tests derived from the work-store page: 12 merge-rule tests (one per items-field row), the list rows, close beats claim, add/add, a cycle made across clones and a conflict no rule settles through two clones, plus the id and ready/blocked/cycle tests.
- **Met:** the 2-clone sync tests against a bare remote pass (13): concurrent edits merge per the table and both stores end equal; concurrent child mints get distinct ids (`.3` and `.4`); after 3 rejected pushes a create fails with nothing written.
- **Met:** the PR, [#87](https://github.com/Yeeef/yeeef-agents/pull/87), is on main as `469930d`.
