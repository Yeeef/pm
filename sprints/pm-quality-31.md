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

::: decision {source=owner date=2026-10-14}
On macOS the 9 RACE_TESTS run only under -race; Linux keeps both runs
They are 47% of internal/work macOS CPU, about 20 s per PR; a non-race-only macOS bug is unlikely and Linux still runs them both ways
Answers `pm-d2k5.31.4`.
:::

::: decision {source=agent date=2026-10-14}
Running OS-independent tests on Linux only moves from Out to In: a PR runs on macOS the darwin build plus the OS-dependent tests; every push to main runs the full macOS suite.
The pm-quality project decision of 2026-10-14 (owner) revises macOS on every PR: most tests generalize across OSes, and a post-merge full macOS run catches what the subset misses.
:::

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

- Levers ranked by measured saving on macos-14 (warm cache, each phase alone,
  PR 81 before and after; before/after CPU in s): (1) the 9 RACE_TESTS run
  twice on macOS: 58.9 s of internal/work's 125.4 s CPU; out of scope, raised
  as a decision need. (2) 4 fewer Dolt links: the item model moved to
  internal/item, which links no Dolt, and records, store, design and site
  import it; their test binaries stop linking Dolt (5 test binaries still
  link it: cli, config, install, service and work, beside pm). Every package's
  tests but internal/work's: 31.0 s wall, 63.3 CPU before; 26.2 s, 48.0 CPU
  after. TestTheItemReadersLinkNoDolt holds it. (3) newPair seeds its 5 items
  locally and pushes them in one sync, as reseed does: 241 to about 70 git
  calls per pair, 10 tests. internal/work's git calls 7599 to 5803 on macOS
  (8148 to 6446 locally); its 8 shards 50.0 s wall, 101.6 CPU before; 37.6 s,
  74.1 CPU after. The race shards 38.1 to 35.5 s. Rejected: test binaries
  linked with -linkmode=internal or -s save 1-2 s CPU of a 7-9 s Dolt link;
  moving config's test to install would drop one more link but needs
  config.Keys exported for a test; a git call's cost does not depend on the
  parent's heap. Locally (32 cores), make test-go ran its parts one after
  another: 35.5-35.8 s at main, 33.9-34.8 s with (2) and (3), 19.6-19.8 s with
  its parts run at once (3 runs each).

- No test lost: go test -v at origin/main (e3ebec5) and at the PR's head, both
  in one CI run per job (run 38530785456). macOS: every package but
  internal/work's 183 results at main (122 top-level, 3 skip), 184 at head
  (123 top-level); internal/work 92 results (32 top-level, 3 skip) both; race
  service 39 (31 top-level, 1 skip) and race work 16 (9 top-level) both.
  Linux: 183 against 184 (2 skip), 92 and 92, 39 and 39, 16 and 16. The sorted
  (package, test, result) lists are identical but for the added
  TestTheItemReadersLinkNoDolt, on both OSes.

- Done-when samples, from the jobs API (macOS = the build-test and race steps
  summed; wall = first job created to last check done). #81 at 280dfb8: 178 +
  70 = 248 s, wall 249 s (last check the agents-md cases job, run because
  AGENTS.md changed; every macOS test ran 1.3-1.6x slower than in other runs
  that hour). #82 at 7595b37: 108 + 58 = 166 s, wall 164 s; it restored main's
  cache from before #81, so it compiled #81's packages. #82's Go jobs rerun on
  main's new cache: 102 + 58 = 160 s, pm-go wall 145 s. Before: 148-181 s over
  13 runs, median 163 s. So macOS Go compute did not measurably fall in CI,
  against 51-55 s targeted. In those suites internal/work's shards took 37.4
  and 41.5 s against 49.7 s before, as the profile predicted. Every package's
  tests but internal/work's took 51-60 s, though, against 26 s when the same
  command ran again right after. The first go command after the cache restore
  pays for reading the restored packages, so the link work cut there is not
  what sets that phase. Two PRs and one rerun were sampled, not three PRs: no
  other session opened a PR, and the one candidate follow-up (moving config's
  test to install) was dropped.

- Classification (table below; .github/scripts/go-os-tests.txt on main holds
  it with the same reasons): of 155 top-level Go tests, 85 can behave
  differently by OS and 70 cannot. Evidence for os: process groups (Setpgid,
  kill(-pgid)); flock; unix sockets and the 104-byte sun_path limit; symlinks
  and realpath (EvalSymlinks, os.Root); file modes and Chmod 0; service units
  (PlatformKind, launchctl, systemctl); XDG data dirs; /proc against ps; exec
  of downloaded binaries; and the embedded Dolt engine writing a store on
  disk. Every package is all os (ask, store), all any (root, cli/parse,
  config, design, hooks, item) or mixed per test (cli, install, launch,
  records, service, session, site, sync, work). Checked: the test set read
  from source equals go test -list's (155 = 155). Each mode's selection equals
  its intended set, run locally with -v: Linux and main-push macOS run all
  155; macOS runs internal/work's 9 RACE_TESTS only under -race; a PR's macOS
  jobs run exactly the 85 os tests (59 outside internal/work, 19 in it, and 7
  race tests under -race; internal/service's os tests also run under -race).
  Main's push run on #83 (ad5659f) ran the full macOS suite and passed: every
  package, 23 internal/work tests, and the 9 race tests plus internal/service
  under -race (build-test 81 s, race 49 s).

- Done-when samples on the new layout. Code PRs whose CI ran with #83 (macOS =
  the build-test and race steps summed; wall = first job created to last check
  done): #83 63 + 67 = 130 s, wall 269 s (the agents-md cases job was last,
  4m25s, run because AGENTS.md changed); #84 63 + 59 = 122 s, wall 109 s; #69
  (another session's) 70 + 62 = 132 s, wall 116 s. Medians: macOS 130 s
  against 163 s before (-20%), wall 116 s against 138 s. Targets ≤55 s and
  ≤100 s not met. The race job is now as long as build-test: it links the race
  build of internal/work (about 30 s) and runs 7 Dolt racing tests under -race
  (19-28 s). #84 also cut TestCheckPortTakesItsOwnUnitsPortWhereNothingAnswers
  from 3.0 to 0.3 s by waiting out 0.1 s probes.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

partial: the Go tests do measurably less work on macOS (internal/work's shards use 27% less CPU, and 4 test binaries stop linking Dolt), but macOS CI compute per PR did not halve; it stayed about 160 s.

- #81: the item model moves to internal/item, which links no Dolt, so records, store, design and site test without Dolt. TestTheItemReadersLinkNoDolt holds this.
- #81: newPair seeds locally and pushes once (241 to about 70 git calls per pair). The compare-and-swap refs check moved onto the concurrent-mints case.
- #81: make test-go runs its parts at once.
- #82: internal/work's test seconds were remeasured.
- Open: decision need "Run the 9 race tests on macOS only under -race?", the largest lever left (58.9 s of internal/work's 125.4 s of macOS CPU).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- macOS Go compute (build and test steps of the two macOS jobs, summed) ≤ ~55 s, median of 3 PRs: **not met**.
  - Samples: #81 248 s, #82 166 s, #82's rerun on main's new cache 160 s.
  - Before: 148-181 s over 13 runs, median 163 s.
  - The frame's 96-98 s baseline was the build-test step alone. That step went from 89-113 s to 102-108 s, or 178 s on a slow runner.
  - Only 2 PRs and 1 rerun were sampled (see Findings).
- A code PR's median wall to the last check ≤ ~100 s: **not met**. #81 249 s (the agents-md job last), #82 164 s, #82's Go rerun 145 s; 138 s before.
- The per-OS test lists are identical before and after: **met**, apart from the added guard test.
  - go test -v at main and at the head ran in one CI run per job.
  - macOS: 183/184, 92/92, 39/39 and 16/16 results. Linux: the same counts.
- `make test-go` local wall time reported: **met**. 35.5-35.8 s at main (3 runs); 33.9-34.8 s with #81's test changes; 19.6-19.8 s with the parts run at once (32 cores).
- `make lint`, `make test` (129 passed) and `make test-go` passed locally. CI was green on #81 and #82, each merged through merge_ready.py's command.
