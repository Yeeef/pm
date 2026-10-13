---
type: sprint
title: The race CI job does not flake
bead: pm-d2k5.24
---

## Goal

> What should be true when this sprint ends, and why now?

The race job passes or fails for a reason in the code under test. On PR #61's second push, race (linux-amd64) failed twice with a "close of closed channel" panic inside Dolt's own code, in TestAnOperationOutlastingItsBound/GCRacingSyncsAndCreatesFinishesPromptly. A third run passed, and the test passed 8 of 8 times locally. A flaky gate trains everyone to rerun instead of read.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Reproduce the panic.
- Find whether pm's use of the engine (GC racing sync and create) triggers it, or it is a Dolt bug.
- Fix it in pm, or work around and report it upstream.
- Add a check that would catch it again.

**Out:**
- Other flaky tests, unless the same cause.

## Done when

> What evidence will show the goal is met?

- The test passes 200 of 200 runs under -race, at CI's settings, with the fix.
- The cause and the fix are in Findings, with the upstream issue if one is filed.
- Every existing design page whose area this sprint changes is brought to the new state and listed in Design pages.

## Design pages

> Where is the detail?

- [pm in Go](../design/pm-go.md): the gc bullets under Store access: the service holds the store, on why a collection returns only once its visits to the other sessions have ended.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-13}
The design page work-store stays as it is for this sprint's code changes.
Its Garbage row (DOLT_GC on a schedule, online, session-aware safepoints) still holds; the wait for each visit's end is host detail, on the pm-go page's gc bullets
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Cause: a Dolt bug, not pm's use of the engine. In
  gcctx/gc_safepoint_controller.go (Dolt b130ee82ebe9, unchanged on Dolt main
  2026-10-13), DOLT_GC visits each idle session in a goroutine that calls
  wg.Done() and only then EndOutstandingVisitCall(), which closes whichever
  channel the session's state holds at that moment. The collection's Wait
  returns on the wait group, so a collection begun right after (the test runs
  10 back to back) can store a new channel in that state first; both
  goroutines then close it: 'panic: close of closed channel' at line 86, the
  stack in both CI failures (jobs 115494040739, 115514850412). One DOLT_GC
  makes one Waiter, so only back-to-back collections hit it; the service
  collects every 24 h.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
