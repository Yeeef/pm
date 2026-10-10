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

- Failure rate on main (0665de2), measured here: go test -race -count=100
  ./internal/service gave 0 data races in 100 runs (4 failures, all
  TestServiceInstallStatusRestartAndLogsEndToEnd on port 8002 held by a second
  concurrent run of mine, not this bug). With the fake counting store calls
  after Run returned, TestAReplyIsRefusedWithItsReason showed 0 late calls in
  300 runs and 0 in 300 at -cpu=1: the window is too narrow to hit locally, as
  CI's one failure then pass on rerun suggests.
  TestRunReturnsOnlyOnceTheGoroutinesItStartedEnded holds the writer in Open
  across the pin move and fails 20 of 20 runs against main's run.go.

- With the fix (a7e5ce1): go test -race -tags gms_pure_go -timeout 40m
  -count=100 ./internal/service: ok in 605.9 s, 0 failures, 0 data races (an
  earlier run hit go test's default 10 min timeout at run ~99 with no test
  hung: the package takes ~6 s a run under -race here). The new test passes 20
  of 20. A fresh-context review found the first version canceled a sync or gc
  under way, which returns the client while the host's statement still runs;
  fixed to let them run to their timeouts, and the merge watch now stops
  between PRs.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: `service.Run` returns only once every goroutine it started has ended, so a stopped pm service no longer reaches the work store after its caller closes the store's host.

- Cause: a real bug, not the test. Run returned as soon as the HTTP server stopped. It did not wait for its loops (writer, refresher, merge watch, syncer, collector) or a reply's retry timer, and `pm service run` closes the host right after Run returns.
- Fix: a stop signal the loops watch, then HTTP server `Shutdown` so the requests under way end, then a wait over every spawned goroutine.
  - The reply retry is now a spawned goroutine instead of `time.AfterFunc`.
  - `put` drops a job once Run has stopped; the reply stays in the spool for the next start.
  - The writer takes no further job once stopped. The merge watch stops between PRs.
  - A sync or gc under way runs to its end or its timeout.
- Checks:
  - The test fake counts every store call made after Run returned. Every served test checks that count at cleanup.
  - `TestRunReturnsOnlyOnceTheGoroutinesItStartedEnded` holds the writer in `Open` while the pin moves.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Item | Met | Evidence |
|---|---|---|
| The new test fails on main and passes with the fix | met | `go test -race -count=20 -run TestRunReturnsOnlyOnce ./internal/service` on main's `run.go` (0665de2) with the new tests: 20 of 20 fail ("Run returned while the writer it started was still opening the work store"; "4 calls reached the work store after Run returned"). With the fix: 20 of 20 pass |
| `go test -race -count=100 ./internal/service` is clean | met | At a7e5ce1 with `-timeout 40m`: ok in 605.9 s, 0 failures, 0 data races. Before, on main: 0 data races in 100 runs. With the late-call check, the original test showed 0 late calls in 300 runs, and 0 in 300 at `-cpu=1`. The CI race window is too narrow to hit on this machine, so the deterministic test is the check |
| The PR's CI passes | met | PR #36, all 9 checks pass at a7e5ce1, including both race jobs. PR #36, merged as 41b6696 |
