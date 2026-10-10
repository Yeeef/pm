---
type: sprint
title: Records shared across worktrees
bead: yeeef-agents-9va.9
---

## Goal

> What should be true when this sprint ends, and why now?

Records behave like Beads for agents: one live store per clone that does not depend on branch or worktree, so every `pm` write is immediately visible to `pm show`, `make render` and the owner, whatever branch or worktree the writer is in. Records keep full per-file git history, and `main` carries a copy of them. Observed 2026-10-03: a context-efficiency finding committed on a worktree branch was invisible from the owner's checkout while the need citing it was visible everywhere.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**

- A `records` branch as the single store, checked out as a fixed worktree at `<main checkout>/.records`, built with `git subtree split` so files keep their history.
- `pm` finds the store from any worktree (`git rev-parse --git-common-dir`), `pm where` prints it, and every `pm` write commits on `records` under a lock; `pm commit` for hand edits.
- A git-ignored `records` symlink in each worktree, so readers and `make render` keep using `records/...`.
- A GitHub Action that copies `records` into main's `records/` on every push to main, with per-file history.
- A check that fails code-branch commits editing `records/`.
- One setup command per clone; pushing and pulling the `records` branch across machines.
- Migrating the context-efficiency records from branch `context-efficiency-1`.
- RULES.md, CLAUDE.md setup and the design page updated.

**Out:** moving records into Dolt or Beads; copying each PR's own record changes to main.

## Done when

> What evidence will show the goal is met?

- A `pm finding add` from a worktree on branch X shows up in `pm show` and `make render` from the main checkout on branch Y without any merge; a `make test` case shows this.
- Two concurrent `pm` writes from different worktrees both land as separate commits on `records`, neither lost; a test shows this.
- Existing records keep their per-file git history on the `records` branch.
- After a PR merges, CI shows main's `records/` equals the `records` branch with per-file history; a code-branch commit that edits `records/` fails the check.
- On a fresh clone, one documented command sets up `.records` and the symlink, and `pm show` works.
- RULES.md, CLAUDE.md setup and the pm-harness design page describe the new layout.

## Design pages

> Where is the detail?

- [Shared records store](../design/records-store.md)
- [pm CLI](../design/pm-cli.md): `pm where` and `pm commit`

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Cross-branch drift also blocks writes: on context-efficiency-1, pm task add
  refused because need 9va.5.5 was closed in Beads while its citing decision
  existed only on dogfood-pm-harness; a .9.3 test should cover this.

- Once main tracks its copy of records/, the records link only works if that
  worktree hides records/ with sparse checkout and sets
  sparse.expectFilesOutsideOfPatterns; without it git 2.37 sees the store's
  files through the link, clears skip-worktree and reports every record as
  deleted. pm setup does both; it also turns on extensions.worktreeConfig for
  the clone.

- Copy-to-main, simulated on a bare clone of this repo with the workflow's own
  run block: after PR 6 merges, main has no records/, so the first run takes
  the git subtree add path; later runs take git subtree merge; a run with
  nothing new is a no-op. Each run left main:records equal to the records tree
  (7fa5112, then 53898b5 after one more store commit).

- Per-file history on main is partial: git blame main --
  records/sprints/pm-harness-9.md attributes lines to the store's original
  commits, but plain git log -- records/<file> on main shows only the copy
  commits (and GitHub's file history with it), because the store's commits
  hold the file at sprints/<file>, not records/<file>. Full per-file history
  is git log records -- <file>.

- Review caught that pm setup only hid records/ when it was already tracked,
  so the first pull after the copy Action would have silently replaced the
  records link with main's lagging copy in every set-up worktree (reproduced
  on git 2.37.3). Setup now always sets the sparse exclusion;
  test_link_survives_main_starting_to_track_its_copy fails on the old setup
  and passes now.

- A worktree-isolated session cannot hand-edit records: the sandbox refuses
  edits outside the worktree, including through the records symlink and
  .records, and pm has no write path for hand-edited sections (Scope, Done
  when, delivery report). Seen 2026-10-04 framing context-efficiency sprint 1;
  pm need respond and pm finding add worked, the Scope/Done when edit did not.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Partial: records live on one shared `records` branch that every worktree
reads and writes without merges, and main carries a CI copy; `pm show` on a
fresh clone was not checked, because that also needs `bd init`, which moves to
sprint 10.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- A `pm finding add` from a worktree on branch X shows up from branch Y
  without a merge: met. `test_write_on_one_branch_is_visible_on_another_without_merge`;
  run for real from a scratch worktree on branch `store-check`, with the
  finding visible in `pm show` and `make render` in the main checkout.
- Two concurrent writes from different worktrees both land: met.
  `test_concurrent_writes_from_two_worktrees_land_as_separate_commits`, which
  failed 3 of 3 runs with the lock removed.
- Existing records keep their per-file history on `records`: met. Built with
  `git subtree split` (63 commits); its tree equalled the last committed
  `records/` (`cc05e2e`).
- After a PR merges, main's `records/` equals the `records` branch and a
  code-branch edit of `records/` fails: met, with history amended by the
  owner's decision on need `9va.9.8` (blame only on main). The copy Action
  ran on the PR #6 merge and `origin/main:records` equals `origin/records`
  (tree `af10d87`); the records guard failed throwaway PR #7, which edited
  `records/`, and passed PR #6.
- On a fresh clone, one documented command sets up `.records` and the link,
  and `pm show` works: partly met. On a simulated fresh clone of merged main,
  `bin/pm setup` checked out `.records`, linked `records`, left `git status`
  clean, and `pm where` worked; `pm show` was not run, because Beads needs
  `bd init` first. A new worktree gets its link from the `post-checkout` hook
  only once `bd init` has installed the hooks. Folding `bd init` into setup is
  sprint 10 (`9va.10.1`).
- RULES.md, CLAUDE.md setup and the pm-harness design page describe the new
  layout: met (commits 5b9d175, 0515711; design pages e20a7dc, ab89118,
  7aca9c9 on `records`).
