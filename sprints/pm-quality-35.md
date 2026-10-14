---
type: sprint
title: The code and test metrics doc measures current main
bead: pm-d2k5.35
---

## Goal

> What should be true when this sprint ends, and why now?

The pm code and test metrics doc describes current origin/main: every table, figure and reading in it is measured at one commit of today's main. It last measured PR #53 (2026-10-12); about 35 PRs have merged since, the CI cuts of sprints 30 to 32 among them (median CI wall per PR 2.9 min then, 1.7 min now), so its numbers no longer say what pm looks like.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** rerun every step of `pm-quality-metrics.py` (size, complexity, lint, tests, runtime, coverage, ci, trend) at one origin/main commit; redraw the figures; rewrite the doc's header, tables and readings to the new numbers; extend the trend with the pm-quality sprints closed since.

**Out:** new metrics, or a change to the method beyond what the generator needs to run on today's tree; acting on what the numbers show.

## Done when

> What evidence will show the goal is met?

- The doc's "Commit measured" names an origin/main commit of 2026-10-14 or later, and every table and figure comes from that run's data.
- `pm commit` of the doc and its figures passes, and the site page https://pm.yeeefs.com/docs/2026-10-12-pm-quality-metrics.html shows the new commit and CI numbers.

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

- Generator fixes for 8ad2720 (method unchanged): (1) trend_commits matched
  only "… (pm-quality sprint N) (#PR)" subjects and dropped sprints 22, 26, 27
  and 28, whose subjects start "pm-quality sprint N: …"; it now takes the
  sprint number anywhere and the PR from the trailing (#N): 24 commits instead
  of 17. (2) runtime read test-go's parts from "test-go: a b", but test-go is
  now a "$(MAKE) -j a b" recipe: it reads either. (3) The CI figure marked #48
  #43 #44 #53, none of them in the new window: it marks #71 #81 #83 #85 (the
  CI sprints' merges) and builds its caption from the marks. (4) Drawing only:
  the trend figure goes one column when it has over 13 commits (24 x labels
  overlapped in a half width), and the CI figure skips a date label that would
  overlap the next.

- CI wall per PR (push to last check, first attempt): median 2.9 -> 1.7 min
  over the last 5 merged PRs (#53 -> #84-#88) and 4.0 -> 2.3 min over the last
  30 (#57-#88). macOS queue is no longer the tail: median 8 s, p90 26 s, max
  101 s over all 30 PRs (was 170-309 s at p90 per macOS job). The tail is now
  the agents md / cases job (the AGENTS.md grader, model calls): 246-306 s on
  #62, #80, #81, #83, #85, which set the 5 slowest walls (4.2-6.4 min). macOS
  race job run median 100 s before #81 -> 47 s over #84-#88.

- Test counts at 8ad2720 vs fe05966: Go top-level tests 89 -> 155 (152 pass, 3
  skip), subtests run 103 -> 121; pytest collected 151 -> 359: light 114 ->
  296 (135 pass, 161 skipped: the 148 owner-request live cases and 13
  AGENTS.md grader live cases), integration 37 -> 63 (12 of them the parity
  live eval).

- Local runtime (go build cache warm, GOFLAGS=-count=1): make test 7.7 -> 12.6
  s wall (load 8.3 before; a second run at load 5.1: 13.0 s); make test-go
  70.5 -> 20.5 s (load 10.1; the second run failed after 19.7 s at 5.7, one of
  the failures below). go-test-work 27.7 ->
  12.6 s; make test-go keeps 4.3 cores busy (was 1.3).

- make test-go fails intermittently at 8ad2720: 4 of 7 runs of
  GOFLAGS=-count=1 make test-go exited 2, each on
  TestServiceInstallStatusRestartAndLogsEndToEnd in go-test-race-service
  (lifecycle_test.go:249, ":8002 is held by another store", a different
  TestServiceInstallStatusRestartAndLogsEndToEnd temp store each time). make
  test-go runs the service tests twice at once (go-test-but-work and
  go-test-race-service under -j); CI runs them in separate jobs. Six runs of
  go-test-race-service alone and three runs of the test in two concurrent
  copies all passed, so the trigger is not fully established. Not fixed here
  (sprint scope Out).

- Coverage (Go tests plus light harness, union): 72.8% of 12,465 -> 74.0% of
  13,925 statements; Go tests alone 55.1 -> 57.0%, light harness 44.4 ->
  44.9%. internal/cli 59.5 -> 69.2% (now 3,476 statements after the split);
  the new internal/cli/setup (pm init, doctor, upgrade, uninstall: 651
  statements) is 0.0% in both, as only integration tests run it. One light
  test fails only under the coverage build
  (test_a_hook_stopped_mid_judge_takes_its_judge_with_it[SIGTERM]: exit 143,
  not 1), because the build's SIGTERM patch exits first.

- Lint backlog (ratcheted linters, whole tree) 481 -> 354: errcheck 364 ->
  311, gocognit 33 -> 10, funlen 36 -> 12, gocyclo 26 -> 0 (gocyclo now gates
  every line), reassign 19 -> 19, file length 3 -> 2. Functions over gocyclo
  30 or gocognit 40: 35 -> 10 (non-test 21 -> 4). //nolint directives 20 ->
  49, 25 of them errcheck. make lint exits 0.

- Size: non-test Go code lines 22,532 -> 25,677 (+14%), Go test lines 8,007 ->
  10,468, harness 4,347 -> 8,148 lines in 28 files; Go packages 18 -> 25.
  Non-test functions over 100 code lines 17 -> 2, the longest 204 -> 102. Over
  the trend, gocyclo > 30 went 17 non-test / 8 test -> 0 / 0 between the
  merges of #80 and #81, the span that holds only #62 (internal/cli split by
  noun).

- Reruns: 7 of 507 pull_request runs since 2026-10-12 failed, then passed on a
  rerun (1.4%; was 7 of 457, 1.5%): agents md cases 3, pm go 3 (race
  linux-amd64 2, work linux-amd64 1, 2.8% of 108 pm go runs), pm tests
  integration 1.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
