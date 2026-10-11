---
type: sprint
title: Split internal/cli by noun and retire the Python-port leftovers
bead: pm-d2k5.21
---

## Goal

> What should be true when this sprint ends, and why now?

`internal/cli` stops being a god package: 8.9k LOC, 34% of non-test code, `writes.go` 1,463 code lines. Its Python-port leftovers go: the hand-rolled argparse emulation (`parse`, cognitive complexity 105) and `pyjson` where no output depends on them. The trimmed suite (sprint 20) and the lint ratchet (sprint 19) are the safety net.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Split `internal/cli` by noun.
- Break up the 5 longest functions: `fromIssue`, `cmdUninstall`, `parse`, `cmdSprintClose`, `mergeItem`.
- Replace the argparse emulation with pflag plus explicit checks where no user-visible output depends on it.
- Retire `pyjson` where no output depends on it.
- Lower the package-size allowance and the ratchet counts to the new state.

**Out:**
- Behaviour changes.
- Clearing the errcheck backlog outside the touched code.

## Done when

> What evidence will show the goal is met?

- No file in `internal/cli` is over 800 lines.
- No function has gocyclo over 30.
- `internal/cli`'s package-size allowance has dropped.
- `make lint`, `make test` and `make test-go` pass, and CI is green.

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
