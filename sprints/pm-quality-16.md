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

- [pm in Go](../design/pm-go.md): Store access, the version handshake table and the paragraph under it

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

- Before this sprint the handshake already refused an off-pin checkout, but
  its fix was 'run it from a checkout that pins pm S' (no rebase named), it
  claimed the main checkout pins S even when the service was stale on a third
  version, and pm where's service line in such a checkout said 'stale: pm was
  replaced since it started; run pm service restart', which restarts the
  service on the same main pin and fixes nothing; pm doctor's service drift
  said the same.

- Fresh-context review of PR #37: no high or medium findings; two low ones
  fixed in 2a0fd00 (a worktree pinned past the move was told to rebase, which
  would not help it; a worktree git could not read failed pm upgrade after its
  writes). Harness pin-move test: 1 passed in 1.80s; touched integration tests
  (-k 'strands or worktree or upgrade or doctor or stale or pin or where'): 41
  passed; make test: 145 passed, 63 skipped; make go-vet go-test go-test-race:
  all ok. .go/pm-before adds one link step to make go-build (0.17 s on a warm
  cache).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: a pin move now names the worktrees it strands, and a stranded worktree's pm where, pm doctor, pm init and every work-store refusal name the mismatch and the fix (PR #37, pending merge).

- `pm upgrade` lists the clone's other worktrees whose branch pins another pm, each with its fix: "merge or rebase it onto the pin move", or, for one pinned past the move, that it runs once its own move is on main.
- The version handshake says which side of a pin move a checkout is on (`git rebase <remote>/<main branch>`, or wait for the merge), and no longer claims the main checkout pins the service's version when it does not.
- `pm where`'s service line and `pm doctor`'s service drift no longer call a service on the main checkout's pin stale from an off-pin checkout, so they no longer send that checkout to `pm service restart`, which fixed nothing.
- `make go-build` also builds `.go/pm-before`, so the harness runs a branch from before a pin move with the pm that branch pins.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Harness test with two worktrees, one rebased onto the pin move and one not: met. `tests/test_config.py::test_a_pin_move_names_the_worktrees_it_strands`:
  - Before the move, `pm upgrade --to VERSION` lists both worktrees.
  - After the move is on main, the service runs VERSION and the fresh worktree is rebased; then `pm upgrade` lists only the stale one, plus a worktree pinned further ahead, which gets its own fix.
  - In the stale worktree, `.go/pm-before` `where` prints the mismatch and the fix on its work line, `doctor` exits 1 with the same line and `show` is refused with it.
  - Result: `make test-full ARGS="-k strands"` 1 passed in 1.80s.
- `make test`, `make test-go`, the PR's CI and `make merge-ready`: met.
  - `make test`: 145 passed, 63 skipped.
  - `make go-vet go-test go-test-race`: every package ok.
  - The integration tests the change touches: 41 passed.
  - PR #37 CI on 2a0fd00: light, integration, changelog, and build-vet-test, work and race on linux-amd64 and darwin-arm64, all passed.
  - `make merge-ready PR=37`: "PR #37 is ready: its head 2a0fd00 contains origin/main 41b6696, and every check on it passed."
- Not checkable before a release: the change helps only a worktree whose pinned release contains it, so the first pin move it covers is the one from the release that ships it to the next.
