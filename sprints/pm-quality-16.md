---
type: sprint
title: A pin move names the worktrees it will strand, before it strands them
bead: pm-d2k5.16
---

## Goal

> What should be true when this sprint ends, and why now?

Moving a repo's pin never surprises an open worktree: `pm upgrade --to X` lists every worktree of the clone whose branch will still pin the old version once the service runs X, with the command that fixes each, and `pm where` and `pm doctor` in such a worktree say so before any work-store command is refused. From pm feedback 2026-10-10 13:25 UTC (pm-quality): moving 0.3.0 to 0.4.0 while ten agent worktrees were mid-sprint refused every pm command in each of them after the service restart, which only the upgrade guides' prose now mentions.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm upgrade --to X` prints the clone's worktrees (from `git worktree list`) whose checked-out branch pins another version, with "merge or rebase onto the pin move" for each.
- `pm where` and `pm doctor`, run in a worktree whose pin differs from the running service's version, name the mismatch and the fix (rebase onto main, or `pm service restart` in the main checkout), instead of only the handshake refusal on the next work-store command.
- Tests for each, and an entry file in `changelog.d/`.

**Out:**
- Letting two pm versions share one service.
- Rebasing worktrees automatically.

## Done when

> What evidence will show the goal is met?

- A harness test with two worktrees, one rebased onto a pin move and one not: `pm upgrade --to X` lists only the stale one, and `pm where` in it names the mismatch while the service runs X.
- `make test`, `make test-go` and the PR's CI pass, and `make merge-ready` says ready.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-10}
pm doctor keeps exiting 1 in a checkout whose pin is not the version the clone's pm service runs; its work store line names the mismatch and the fix
The work store is a piece of the clone's setup pm doctor checks through the service, and from an off-pin checkout the handshake refuses that check; exit 0 would claim a setup it could not check. The release line, which concerns no piece, stays informational
:::

::: decision {source=agent date=2026-10-10}
pm upgrade leaves the main checkout and the records store off its stranded-worktree list, and lists on every run, not only when it moves the pin
The service follows the main checkout's pin, so the main checkout is the one the move reaches, never stranded; a second run after a merge (nothing to commit) is how an agent checks which worktrees are still off the pin
:::

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
