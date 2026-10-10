---
type: sprint
title: pm clean removes agent worktrees nobody needs
bead: pm-d2k5.6
---

## Goal

> What should be true when this sprint ends, and why now?

The owner or an agent runs `pm clean` and every agent worktree that no live session owns and whose work is saved is removed, while anything live, dirty or unsaved is kept with the reason printed.

Moved from pm-harness sprint 55 on 2026-10-10.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** a `pm clean` command, a dry run by default and `--apply` to act. It finds owners from Claude Code's worktree lock (pid plus process start time) and from transcript activity in the worktree's `~/.claude/projects/` dir within `LIVE_WINDOW`. It removes a worktree only when it is clean and its branch is on `origin/main` (ancestor, or a squash merge checked with `gh`, kept when `gh` is unavailable) or has nothing beyond its pushed upstream. It deletes local branches only once merged, and uses `git worktree remove` without `--force`, then prunes. Tests in a temp repo cover each case.

**Out:** flagging leftovers from the scheduled `pm push` job or in `pm show`; Codex worktrees; reading macOS lock start times (a live pid counts as owning the worktree there).

## Done when

> What evidence will show the goal is met?

- `pm clean` lists each worktree with keep or remove and a reason; `--apply` removes exactly the remove set and never the main checkout, the records store (`.pm/store/records`) or the calling worktree.
- Harness tests cover dirty, unpushed commit, merged, squash-merged, live lock, stale lock and the store, and `make test` passes.
- A run on this clone removes the merged leftovers and keeps any worktree with a commit never pushed.

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

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
