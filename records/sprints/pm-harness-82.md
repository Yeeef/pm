---
type: sprint
title: Keep agents off the unfiltered integration run
bead: yeeef-agents-9va.91
---

## Goal

> What should be true when this sprint ends, and why now?

Agents stop running the whole integration set locally: CI runs it on every PR, so a local full run spends minutes and proves nothing CI will not.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** a Makefile guard that refuses `make test-full` without ARGS outside CI; pm/AGENTS.md and the Makefile comments say plainly not to run the whole set and that CI is the check.
**Out:** renaming the target; changing the CI workflow.

## Done when

> What evidence will show the goal is met?

`make test-full` with no ARGS exits non-zero with a message naming the alternatives; `make test-full ARGS="-k <x>"` and `CI=1 make test-full` still run; `make test` passes; the PR's CI is green.

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

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: a bare `make test-full` now refuses outside CI, and pm/AGENTS.md says the PR's CI run is the integration check.

Merged as 16be954 (PR #78).

- Makefile guard (exit 2, names `ARGS="-k …"`, `gh pr checks --watch`, `CI=1`)
- pm/AGENTS.md test rules and the pm-tests workflow comment reworded

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Bare `make test-full` exits non-zero with the alternatives: met, exit 2.
- `ARGS="-k serve"` and `CI=1` still run: met, `make -n` prints the unchanged pytest command.
- `make test` passes: met, 105 passed, 35 skipped.
- PR CI green: met, PR #78 light, integration and guard jobs pass.
