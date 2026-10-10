---
type: sprint
title: The pm service's goroutines end when Run returns
bead: pm-d2k5.15
---

## Goal

> What should be true when this sprint ends, and why now?

When `service.Run` returns, no goroutine it started still touches the work store, so a stopped service never opens the store behind a caller that believes it is down. Found on PR #34's CI (2026-10-10): `race (linux-amd64)` failed in `internal/service` `TestAReplyIsRefusedWithItsReason`; the test cleanup read `w.open` (run_test.go:106) while the writer goroutine, through `pushUndelivered` → `withStore` → `fakeWork.Open` (fake_test.go:49), was still opening the store after `s.stop()`. It passed on rerun.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Find which goroutine outlives `Run` and make `Run` wait for it (or cancel it) before returning.
- A test that fails when a goroutine started by `Run` is still running or touches the store after `Run` returns.

**Out:**
- Other test-fake races (pm-quality sprint 13 fixed the one it found).

## Done when

> What evidence will show the goal is met?

- The new test fails on main and passes with the fix; `go test -race -count=100 ./internal/service` is clean; the PR's CI passes.

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

- Cause: a real bug, not the test stopping the wrong thing. service.Run closed
  its done channel and returned as soon as srv.Serve returned, without waiting
  for the five loops it started (writer, refresher, ticker, syncer, collector)
  or a reply's time.AfterFunc retry; internal/cli/serve.go then closes the
  work store's host (defer h.Close()), so a writer mid-job
  (deliverReply/pushUndelivered -> withStore -> Open) could reach a closing
  store. The test-side race (cleanup reading w.open without the fake's lock)
  only surfaced it.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
