---
type: sprint
title: "The Go tests do less work: macOS CI compute halves"
bead: pm-d2k5.31
---

## Goal

> What should be true when this sprint ends, and why now?

The free macOS runner's 3 cores spend about 96–98 s per PR on Go work, plus 25–35 s of checkout and cache restore, so a code PR takes a median 138 s (sprint 27). Rearranging jobs no longer helps; the work itself must shrink: what is compiled, linked and set up per test. Target: macOS Go compute about 50 s, and a code PR about 100 s.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Profile macOS work per package: compile, link and test run time, plus per-test CPU from `go test -json`, both on CI and locally. The question is where the 96–98 s goes.
- Cut link work: count how many test binaries link Dolt and fold or restructure them where that cuts links, for example by keeping store-free packages from importing it.
- Make fixtures cheaper: a store or clone template built once per package and copied per test, instead of created per test, and less git subprocess work per test.
- Remove waits: sleeps and backoffs that set test time, without weakening what a test checks.
- The same changes speed up `make test-go` locally.

**Out:**
- Running OS-independent tests on Linux only. That changes the owner's "macOS on every PR" decision; if profiling shows it is the main lever, raise it as a decision need with the numbers.
- Paid or self-hosted runners.
- Dropping any test or assertion.

## Done when

> What evidence will show the goal is met?

- macOS Go compute (the build and test steps of the two macOS jobs, summed) is ≤ ~55 s, from 96–98 s, the median of 3 PRs.
- A code PR's median wall time to the last check is ≤ ~100 s over 3 PRs, from 138 s.
- The per-OS lists of tests run are identical before and after.
- `make test-go` local wall time is reported before and after.

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

- Profile, macos-14 (3 cores, Apple M1 virtual), PR 81's first head; local
  runs on 32 cores under taskset -c 0-2. Baseline: the two macOS jobs' build
  and test steps summed were 148-181 s over 13 recent runs (build-test 89-113
  s, race 51-80 s); the frame's 96-98 s is the build-test step alone.
  Build-test's 113 s: the pm build beside every package's tests but
  internal/work's 55 s, work's test binary 8 s, work's 8 shards 50 s (101.6 s
  CPU: 49.6 user, 52.0 sys). Compiles come cached from main's cache;
  recompiling our own packages costs 22.6 s of compile in 50 actions. 10
  binaries per build-test run link Dolt: pm, work.test and 8 test binaries
  (cli, config, design, install, records, service, site, store). A Dolt link
  takes 4.8-7.7 s wall and 7-9 s CPU on macOS, 1.8-2.3 s locally. records,
  store, design and site link Dolt only through work's item types; config only
  through its test's import of install. Per test, each run alone: 158.7 s CPU
  on macOS against 34.2 s locally; internal/work is 125.4 s of it (61.5 user,
  63.8 sys), with 7599 git subprocesses (8148 locally). A git call costs 4.9
  ms wall and 4.5 ms CPU on macOS (homebrew git; /usr/bin/git 13 ms) against
  0.7 ms locally; the parent's heap does not change it (0, 256 or 1024 MB).
  Top tests by macOS CPU, s (git calls): TestCreateRacingALocalWriter 30.9
  (1022), TestPullRacingLocalWriters 17.2 (770), TestCreatePushTimeout 11.8
  (972), TestSyncTwoClonesMergeListFieldsPerEntry 11.6 (825),
  ...CloseBeatsClaim... 9.8 (782), ...MoveSprint... 9.1 (731),
  ...ConcurrentEdits... 6.9 (527), cli
  TestStampAndTextsSurviveGitRewritingTheRecordsMidWalk 6.8 (50),
  TestSyncFailsHardOnAnUnsettledConflict... 6.2 (560),
  TestAnOperationOutlasting... 5.7 (332), TestSyncFailsHardOnAddAddAndACycle
  5.7 (517), cli TestLoadNeverPasses... 5.2 (155),
  TestTwoClonesMigratingOnTheirOwnMerge 3.4 (201),
  TestCreateRefusesWithTheRemoteUnreachable 2.8 (244), install
  TestSetupWorkCreatesPushesAndClones 2.0 (80), TestSetupWorkRefusesAFork 1.8
  (72), site TestCheckRefusesEachBrokenRecord 1.6 (96), cli TestSyncCommand
  1.4 (58), TestCommentsKeepTheirOrderWithinASecond 1.3 (109),
  TestAWriteRacingOneOutsideTheLockFailsHard 1.2 (0). The racing tests loop
  their writers for as long as the raced operation takes, so their CPU grows
  with the machine's slowness (10x on macOS). The two-clone fixture newPair is
  241 git calls (a create through the remote about 50, a pushing sync 52, a
  clone 20), and 10 tests build one. The 9 RACE_TESTS are 58.9 s of work's
  macOS CPU and run again in the race job. Dolt's 1.1 s read-dedup waits and
  the other sleeps are wall, not CPU: with the 3 cores saturated they do not
  set macOS time.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
