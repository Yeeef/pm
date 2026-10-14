---
type: sprint
title: make test-go passes on every run
bead: pm-d2k5.36
---

## Goal

> What should be true when this sprint ends, and why now?

`make test-go` passes on every run at main. At `8ad2720` it failed 4 of 7 runs on a 32-core machine (sprint 35's findings), each time in `go-test-race-service`; the 3 failures with a kept log failed `TestServiceInstallStatusRestartAndLogsEndToEnd` with ":8002 is held by another store", the store being another copy of the same test. A local check that fails half the time is not a check.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** reproduce the failure; find why two runs of the service tests clash on one port under `make -j` (`go-test-but-work` and `go-test-race-service` both run them); fix the cause in the tests, the Makefile or the code, with a test or check that would catch it coming back.

**Out:** other flaky tests; CI changes, unless the cause is shared with CI.

## Done when

> What evidence will show the goal is met?

- The failure reproduces at least once at `8ad2720` before the fix, with the command and its output in Findings.
- At the fix's commit, `make test-go` passes 10 runs out of 10 on the same machine, with the load at each start recorded.
- The fix's PR has green CI and a review need raised.

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
