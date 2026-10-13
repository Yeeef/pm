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

- Reproduced at CI's settings (64 -race processes at once: 8 per 4 pinned
  cores, GOMAXPROCS=4, as 8 shards on a 4-vCPU runner): the flaky test
  panicked 1 of 3000 runs before the fix (1/1000, then 0/2000 with 7 other
  failures, all 'slowest of 10 collections took 5.0s' while make lint and make
  test-go ran on top), and 0 of 1000 at GOMAXPROCS=2 (4 per 2 cores). An
  amplifier (32 idle sessions, 60 back-to-back collections) panicked 607 of
  1000 runs before the fix, and 0 of 200 without oversubscription.

- Fix in pm: Host.GC, after CALL DOLT_GC() and still under gcMu, waits for
  each session's last visit to end (internal/work/host_gc.go reads the
  controller's unexported mu and sessions by reflection; the host's start
  fails if a Dolt upgrade changes them). After the fix: the flaky test passed
  2000/2000 (4 cores) + 1000/1000 (2 cores) + 1000/1000 on the final commit;
  the amplifier 0/1000. The new check,
  TestACollectionRacingItsOwnVisitsReturnsOnceEachEnds (32 idle sessions, 30
  collections, no visit outstanding when each returns; in RACE_TESTS), fails
  410/1000 under contention and 27/200 without it when the wait is removed,
  and passes 1000/1000 with it. No upstream issue filed; its text is in the
  delivery report.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: the race job's "close of closed channel" panic was a Dolt bug that back-to-back collections hit, and pm now waits for each collection's visits to the other sessions to end before the next collection may begin.

- `Host.GC` waits, under `gcMu`, for each session's last visit to end (`internal/work/host_gc.go`). It reads Dolt's GC safepoint controller by reflection, and the host's start fails if a Dolt upgrade changes the controller's fields.
- New check: TestACollectionRacingItsOwnVisitsReturnsOnceEachEnds, which is in RACE_TESTS.
- PR [#74](https://github.com/Yeeef/pm/pull/74).

Upstream issue text for dolthub/dolt. pm has not filed it.

> **GCSafepointController: a visit's EndOutstandingVisitCall can close the next collection's channel ("panic: close of closed channel")**
>
> In `go/libraries/doltcore/doltdb/gcctx/gc_safepoint_controller.go`, `Waiter`'s visit goroutine runs `ret.wg.Done()` and then `state.EndOutstandingVisitCall()`. That call closes `state.QuiesceCallbackDone.Load()`, the channel the state holds *at that moment*. `GCSafepointWaiter.Wait` returns as soon as the wait group is done. So `dolt_gc` can return, and a following `dolt_gc` can create a new `Waiter`, whose `work()` calls `BeginOutstandingVisitCall()` on the same idle session and stores a new channel, before the first goroutine has run `EndOutstandingVisitCall()`. Both goroutines then close the new channel, and the process panics: `panic: close of closed channel` at `gc_safepoint_controller.go:86`, from `Waiter.func1.1` (line 171). We hit it with an embedded engine that runs `CALL DOLT_GC()` back to back while other connections sit idle. Under the race detector with CPU oversubscription, a loop of 60 collections with 32 idle sessions panics in about 60% of runs.
>
> Suggested fix: capture the channel in `BeginOutstandingVisitCall` and close that one (for example, `ch := make(chan struct{}); state.QuiesceCallbackDone.Store(ch)`, and the goroutine closes `ch`). Or call `EndOutstandingVisitCall()` before `wg.Done()`, so that `Wait` returns only once every visit has ended.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **The test passes 200 of 200 runs under -race at CI's settings, with the fix: met.** These runs used the race test binary with 64 processes at once: 8 per 4 pinned cores, `GOMAXPROCS=4`, which is CI's 8 shards on a 4-vCPU runner.
  - After the fix: 2000/2000, then 1000/1000 on the final commit, plus 1000/1000 at `GOMAXPROCS=2`.
  - Before the fix: 1 panic in 3000 runs, with the CI stack. An amplifier panicked 607/1000 before the fix and 0/1000 after.
  - CI on PR #74 is green, including race (linux-amd64).
- **The cause and the fix are in Findings: met.** No upstream issue was filed; its text is above.
- **Each design page whose area changed is updated and listed: met.** [pm in Go](../design/pm-go.md) has a new gc bullet under Store access and is listed in Design pages. work-store is recorded as unchanged, because its Garbage row still holds.
