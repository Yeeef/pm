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

- Reproduced at 8ad2720: 8 concurrent copies of the race test binary (`go test
  -c -race -tags gms_pure_go -ldflags=-w ./internal/service`, then from
  internal/service 8x `service-race.test -test.run
  '^TestServiceInstallStatusRestartAndLogsEndToEnd$' -test.count 3` at once)
  failed 16 of 24 runs, 7 of 8 copies, each with sprint 35's error:
  "lifecycle_test.go:249: installed local.pm.main.9d6d8751, but the site does
  not answer for this store on :8002 (... down: :8002 is held by another store
  (/tmp/TestServiceInstallStatusRestartAndLogsEndToEnd4038071911/001/main/.pm/store/records)"
  (11 on :8002, 1 on :8003, 4 on :8004). `make test-go` itself passed 6 of 6
  at 8ad2720 here (load 6.0 to 16.4 at the starts): the clash needs both
  copies inside the same ~0.1 s window, which make -j hits only sometimes.

- Root cause: TestServiceInstallStatusRestartAndLogsEndToEnd took its site
  port from service.FreePort, the first free port from 8000 that no pm unit in
  HOME names. Each copy fakes its own HOME, so copies running at once see none
  of each other's units and, with 8000 and 8001 held by this machine's real
  services, all take :8002 in the window between FreePort and the service's
  bind; the second service finds the first's store there. make test-go runs
  internal/service twice at once (go-test-but-work plain, go-test-race-service
  under -race), and other worktrees run it too. Fix: the test takes a
  kernel-picked port (listen on 127.0.0.1:0); the root package's
  TestNoTestTakesFreePortsSitePort parses every Go file and fails on a
  FreePort call in a _test.go file (it fails on 8ad2720's
  lifecycle_test.go:244). After the fix the 8-copy reproduction passed 24 of
  24 at ea50cfd.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
