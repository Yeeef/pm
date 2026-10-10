---
type: sprint
title: "pm CI: a PR's slowest check under 5 minutes"
bead: pm-d2k5.3
---

## Goal

> What should be true when this sprint ends, and why now?

A typical pm PR's slowest check finishes in under 5 minutes, so a sprint's PR is ready for review soon after it is pushed. Why now: PR #7's build-and-parity job took 11m29s on Linux and 14m45s on macOS for a prompt-only change, most of it compiling.

Moved from pm-harness sprint 110 on 2026-10-10.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A Go build cache that is refreshed (today setup-go saves only on a new go.sum, so the cache is from 2026-10-09).
- The release build test run only when the release build can change (release/, go.mod, go.sum) and on tags.
- The race tests and the shared suite split into parallel jobs.

**Out:**
- Making the tests themselves faster (internal/work's 81 s Dolt tests).
- Dropping the macOS runner.

## Done when

> What evidence will show the goal is met?

- Two PRs after the change, neither touching release/ nor go.sum: each one's slowest check under 5 minutes, read from gh pr checks.
- A PR touching release/ still runs the release build test, and passes.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-10}
Fix the GC journal flake by building with Dolt b130ee82ebe9 (dolthub/dolt#11312), guarded by TestDoltHoldsTheJournalPruneFix; pm does not serialize GC with sync.
The race is inside Dolt journal (any journal re-creation during a prune hits it, not only the fetch); upstream fix fails-before/passes-after 20/20; a GC that waits on sync deadlocks, since DOLT_GC waits for sessions mid-statement (host.go:328-332).
:::

::: decision {source=agent date=2026-10-10}
CI runs the Go tests with GOFLAGS=-count=1, and saves the Go caches on pushes to main plus once per PR whose go.sum has no saved cache.
A refreshed cache would otherwise let cached test results stand in for test runs; saving on every PR run costs upload time on each job, while a go.sum change otherwise stays cold for every push of its PR.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- From pm-harness sprint 110: PR #7 Linux build-and-parity, 11m29s, from the
  job log: setup 0:30; build, vet and Go tests 1:56 (internal/work 81 s);
  -race tests 3:47 (tests 6 s and 39 s, the rest compiling with -race); parity
  0:20; make test-go-suite 1:51; release build test 2:56 (release/build.sh
  twice). The setup-go cache hit its primary key and logged 'not saving
  cache'; the cache dates from 2026-10-09, the last go.sum change. That the
  -race compile is uncached is inferred from those dates, not measured.

- Before the change (pm go run 38050732408, PR #13, one job per target): Linux
  9:31 = setup 0:32 (886 MB setup-go cache) + go-build 0:11 + vet and tests
  1:48 (internal/work 81.9 s) + race internal/service 3:01 + race
  internal/work 0:48 (40.0 s of tests) + release build test 3:01; macOS 10:16
  = setup 1:04 (891 MB cache) + go-build 0:25 + vet and tests 2:58
  (internal/work 133.3 s) + race 2:14 + 1:10 (56.2 s of tests) + release build
  test 2:17. The setup-go cache hit its primary key (the go.sum hash) and was
  never saved after 2026-10-09: the non-race build was warm (go-build 11 s),
  the -race compile cold (internal/service: 6.5 s of tests in a 3:01 step). Go
  test results were cached across runs too (pyjson "(cached)").

- The UpdateGCGen fatal is a race inside Dolt, not a GC outliving its test
  (Host.GC is synchronous; cleanup is Close then RemoveAll). In Dolt
  a6690826d767 (pinned), ChunkJournal.PruneTableFiles
  (store/nbs/journal.go:350-361) protects the journal only if a writer exists
  when it starts; a concurrent write that bootstraps the journal
  (journal.go:512, outside pruneMu) gets its file deleted by the prune
  (file_table_persister.go:373-420), its writes land in an unlinked file, and
  the next GC panics in dropJournalWriter (journal.go:455). PR #16 race job
  114223542469 reproduced it in 10.7 s with a DATA RACE report: dolt_fetch (pm
  sync) bootstrapJournalWriter vs GC PruneTableFiles. Upstream fix
  dolthub/dolt#11312 (b130ee82ebe9, 2026-07-17): its regression test fails
  20/20 on the pinned Dolt, passes 20/20 (and 5/5 -race) on the fix. pm level,
  no local repro in 330 runs (0 failures) before; after the bump, -run GC
  -count=30 ok (93.2 s), -race -run GC -count=5 ok (20.6 s). Production
  exposed: the service GCs while commands and syncs write.

- After the change, PR #16 (head b18634a, go.sum changed by the Dolt bump).
  First attempt, caches cold for the new go.sum: build-vet-test 6:11 Linux /
  6:16 macOS, work 4:17 / 5:20, race 2:40 / 5:04, release build 6:00 / 5:21,
  light 2:45, integration 3:26. Rerun with the caches that first attempt saved
  (as the next push of a PR, or any PR after main saves): build-vet-test 1:18
  / 2:11, work 1:55 / 3:47 (macOS internal/work tests 137.4 s), race 1:20 /
  2:07, release build 2:35 / 3:59, light 1:00, integration 1:18. Slowest check
  3:59 (release build, macOS), 3:47 among the checks every PR runs (was 9:31
  Linux / 10:16 macOS). macOS cache restore takes about 1 min of each macOS
  job.

- Path filter checked: draft PR #17 against ci-fast changed only
  release/build.sh (one comment); pm-release-build.yml ran on it (run
  38056855929) and passed, 4:43 Linux / 5:48 macOS, cold. Closed unmerged.

- PR #22 on main 4c36803 (first PR after #16): slowest check build-vet-test
  linux 6m28s, darwin 6m07s, race darwin 5m46s, over the 5-minute goal; likely
  cold caches, since #16 changed go.sum and main's first post-merge run saves
  them. race linux failed once in TestCreateRacingALocalWriter (clone: invalid
  connection, a go-mysql-server caught panic in Dolt's localFS.iter) and
  passed on rerun; watch for it on the new Dolt.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

partial: in PR #16, with warm caches, the slowest check is 3:47 for the checks every PR runs (it was 9:31 on Linux and 10:16 on macOS) and 3:59 when the release build test runs; the steady state across two later PRs can be read only after the merge.

- `pm-go.yml`: three parallel jobs per target: build, vet and every package's tests except internal/work's; internal/work's tests; the race tests. Each job uses the Go caches (GOMODCACHE, GOCACHE). A push to main saves them, keyed by target, Go version, kind and go.sum. A PR whose go.sum has no saved cache saves its own once. Tests run with `GOFLAGS=-count=1`.
- `pm-release-build.yml`: the release build test. It runs only on changes to `release/`, `install.sh`, `go.mod`, `go.sum`, `internal/buildinfo`, the test or the workflow itself, and on every `pm-v*` tag.
- `Makefile`: `test-go` split into `go-vet`, `go-test` (`GO_PKGS`) and `go-test-race`. `make test-go` runs what it ran before.
- The flaky GC fatal was a race in Dolt's journal prune. pm now builds with Dolt b130ee82ebe9 (dolthub/dolt#11312). `TestDoltHoldsTheJournalPruneFix` fails when go.mod pins an older Dolt. CHANGELOG has a Fixed entry, because the pm service was exposed.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Two PRs after the change, neither touching release/ nor go.sum, each with its slowest check under 5 minutes: **not yet checkable**. The sibling sprints' PRs measure it after PR #16 merges and main saves the caches. Expected result, from PR #16's warm rerun (run 38056854222, attempt 2, `gh pr checks 16`): build-vet-test 1:18 on Linux and 2:11 on macOS, work 1:55 and 3:47, race 1:20 and 2:07, light 1:00, integration 1:18. Slowest: 3:47. A PR that leaves release/ and go.sum alone does not run the release build test. The push after the rebase onto main (head 62979dd) had warm caches and measured: build-vet-test 0:52 on Linux and 2:00 on macOS, work 2:05 and 3:14, race 1:22 and 1:56, light 0:58, integration 1:14, release build 3:34 and 4:04. Slowest: 4:04.
- A PR touching release/ still runs the release build test, and it passes: **met**. Draft PR #17 changed only `release/build.sh` against `ci-fast`. `pm release build` ran on it (run 38056855929) and passed: 4:43 on Linux, 5:48 on macOS, cold. PR #16 itself (go.mod, go.sum and the workflow) ran it too: 2:35 on Linux and 3:59 on macOS, warm.
- PR #16, merged as 4c36803 after a fresh-context review (merge; two low gaps carried to sprint 13).
