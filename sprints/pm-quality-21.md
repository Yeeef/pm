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

- [pm CLI](../design/pm-cli.md): command packages, parser stages and permanent limits.
- [pm in Go](../design/pm-go.md): module boundaries and JSON compatibility.
- [pm as an installable product](../design/pm-product.md): setup coverage.
- [Decision need layout](../design/decision-need-layout.md), [Sprint lifecycle](../design/sprint-lifecycle.md), [Session claims](../design/session-claims.md), [Reply delivery](../design/reply-delivery.md): new command-file ownership.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-13}
Apply the cyclomatic-complexity threshold to every Go function, including existing tests.
The Done when says no function exceeds 30; applying it globally gives an exact check and removes ambiguity about the named five.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Baseline: internal/cli has 8,896 non-test Go lines; writes.go has 1,576
  total lines, install.go 1,013, needs.go 950. Before changes, make test
  passed 83 tests, skipped 136, in 11.27 seconds.

- Verification: make test passed 83 tests (136 skipped) in 11.31 s; focused
  setup integration passed 18 tests in 12.69 s. make lint passed all gates and
  the ratchet with zero findings. make test-go passed, including eight race
  tests in eight shards (11.4 s). All rendered site goldens remain unchanged.
  Command package: 7,445 non-test lines (down from 8,896); largest file is
  commands.go at 714 lines. Every Go function is at most cyclomatic complexity
  30, now a global CI gate; every cli Go file is capped at 800 lines.

- Final implementation commit d4acff4 is PR #62. After review restored
  existing best-effort cleanup semantics, make lint passed with zero global
  and ratchet findings; uninstall integration passed 3 tests in 4.87 s;
  focused merge/sync tests passed in 55.665 s. Final light harness passed 83
  tests, skipped 136, in 11.14 s. Fresh review found no correctness issues,
  including a second read of the cleanup patch. Human-facing/stored pyjson
  forms are retained; only machine inbox JSON encoding was replaced. The
  parser already used pflag before this sprint; its remaining normalization
  and explicit checks are now staged in the parse package.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
