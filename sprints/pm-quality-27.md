---
type: sprint
title: A PR's CI finishes in about 90 s and queues less for macOS runners
bead: pm-d2k5.27
---

## Goal

> What should be true when this sprint ends, and why now?

A PR's CI finishes in about 90 s, and two PRs running at once wait less for macOS runners. Today the median wall time to the last check is 115 s on an idle queue, and up to 225 s when sessions overlap. Every GitHub plan short of Enterprise allows 5 concurrent macOS jobs, and each PR starts 3–4 of them, so paying does not help (owner decision 2026-10-13: the free route).

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- macOS `build-vet-test` keeps the OS-dependent work (build, tests) and leaves the OS-independent checks (vet, golden pages) to Linux. It is the longest job, at 95–137 s.
- Fewer macOS jobs per PR: fold work, race and build-vet-test on darwin into one or two jobs.
- Shorten the race floor: `TestPullRacingLocalWriters`, 32 s under `-race`.
- Path filters, so a PR skips the workflows its changed files cannot affect. Required checks must still report.
- `make merge-ready` must keep requiring each always-on workflow.

**Out:**
- Self-hosted or paid runners.
- Dropping macOS from PRs.

## Done when

> What evidence will show the goal is met?

- The median wall time to the last check on 3 separate PRs is ≤ ~90 s.
- Each PR starts at most 2 macOS jobs.
- A PR that changes only `tests/*.py` or docs skips the Go matrix and still passes `make merge-ready`.
- No test or check is dropped: the before and after counts of tests run per OS are equal.

## Design pages

> Where is the detail?

- [Views and site](../design/views-and-site.md): a records read while git holds the store worktree's index lock is repeated under the records lock.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-13}
In scope too: pm-lint and pm-release-build restore pm-go.yml's current cache key (go-unzipped-...), and make lint-deadcode runs its two analyses at once.
Both are on the PR critical path to ~90 s: lint-deadcode ran 67-92 s, restoring a stale pre-unzipped cache that only stays alive because lint keeps restoring it; its two type-checks run serially on 4 cores (10 s each with 32 cores locally, about 35 s CPU each).
:::

::: decision {source=agent date=2026-10-13}
In scope too: the service's lock-free records read (internal/cli serve.go Load) counts a read while git holds the store worktree's index.lock as moved, so it reads again under the records lock.
TestLoadNeverPassesARecordSetMissingAFileGitRewrote failed in 4 of 4 macOS runs with build, tests and work tests at once (-j3), and once on Linux: a git stalled mid-checkout leaves the same file missing from both stamps, so a read passed without it. Under CPU contention locally: 9 of 40 runs failed before, 0 of 40 after. The -j3 layout is what brings the macOS build-test job from 138 s to about 90 s.
:::

::: decision {source=agent date=2026-10-13}
The design page pm-go stays as it is for this sprint's code changes.
Only the records read's index-lock check in serve.go changed; views-and-site holds that.
:::

::: decision {source=agent date=2026-10-13}
The design page site-replies stays as it is for this sprint's code changes.
Only the records read's index-lock check in serve.go changed; views-and-site holds that.
:::

::: decision {source=agent date=2026-10-13}
The design page work-store stays as it is for this sprint's code changes.
Only internal/work tests changed: TestPullRacingLocalWriters skips read-dedup waits before pushes that need no fresh fetch.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- TestPullRacingLocalWriters spent about 2 s a round sleeping out Dolt's 1.1 s
  read dedup before pushes that need no fresh fetch (A's push opening a round,
  B's push after its racing pull: nobody pushed since their last read).
  Skipping those two waits keeps the racing pull, its three cadences and every
  assertion: locally under -race 14.8-15.6 s to 9.5-10.0 s (3 runs each; 6.2 s
  of CPU in 15.3 s wall before). On macos-14 it stayed 21-27 s in both forms:
  there the test is bound by per-operation latency on a busy 3-core runner,
  not by the sleeps.

- macOS cost per job, from job logs (macos-14, 3 cores): checkout, setup_go
  and cache restore 25-35 s (restore 19-28 s: ~9 s download at 50 MB/s, ~15 s
  gtar+zstd extract of ~67k files); pm's link 18-20 s wall on 8-9 s CPU; every
  package's tests but internal/work's 30-47 s; internal/work's 50 s (compile
  7-9 s, 8 shards 33-54 s); the race tests 63-70 s (compile 16-26 s, shards
  28-40 s). CPU from bash time: internal/work's tests 111 s in 50 s (74% of 3
  cores), race 107 s in 70 s. Running build, tests and internal/work's at once
  (-j3) took 96 s against 98 s one after the other: the cores are the bound,
  so 2 macOS jobs cost about 125 s and 105-110 s.

- Rejected by measurement: tests' temp files on a RAM disk (TMPDIR on an APFS
  ram:// volume on macOS, /dev/shm on Linux) made macOS slower (race shards
  37.1 s against 30.5 s on disk; internal/work's 50.9 s against 33.5 s) and
  Linux no faster (work job 42 s against 38 s), so macOS is not fsync-bound;
  restoring both the build and race caches in one macOS job cost 42 s of
  restores (26 + 16 s); running deadcode's two analyses at once took 74 s
  against 61 s serial (240 s of CPU against 169 s on 4 cores); moving the
  golden-page tests off macOS saves under 1 s, so they stay and every OS keeps
  every test.

- No test lost, from go test -v job logs of main's layout (PR 70 at 185c7e0)
  and this one (PR 71): macOS every package but internal/work 163 results (117
  top-level; 160 pass, 3 skip), internal/work 32 tests, race 9 work tests plus
  internal/service's 39 results; Linux 163 (161 pass, 2 skip), 32, 9 plus 39.
  The sorted per-test lists of each OS are identical before and after. Only
  gofmt, go vet and the renderer and pm-before builds left macOS: they read
  the same files (no Go file is OS-specific) and serve the Linux-only harness.

- pm lint restored a stale cache: its key lacked pm-go.yml's 'unzipped' form
  since sprint 18, so it matched go-linux-amd64-...-fe05966, kept alive only
  by these restores. And TestLoadNeverPassesARecordSetMissingAFileGitRewrote
  was a real race: a git stalled mid-checkout leaves the same unlinked record
  missing from the read and both stamps, so the service's lock-free read
  passed without it. It failed 4 of 4 macOS runs with three makes at once and
  once on Linux; under 2 cores with 6 busy loops locally, 9 of 40 runs failed
  before the index.lock check and 0 of 40 after.

- Done-when samples on the new layout. Wall is from the first job created to
  the last check done; one run per PR. #71 (head ddf25f5): 138 s, macOS
  build-test 130 s and race 109 s; its earlier heads took 165 s and 107 s. #77
  (docs only): 88 s, with the Go jobs skipped in 4-11 s (go_changes: go=false,
  AGENTS.md) and bounded by the harness's integration job at 85 s. #79
  (.github/scripts only, Go runs): 141 s, and 170 s after its rebase. Median
  of the three PRs: 138 s. Each started 2 macOS jobs. #78 started 3:
  pm-release-build.yml is path-filtered by design and ran for its own change.
  For comparison, other sessions' PRs on the old layout tonight: 109, 202 and
  228 s with 3-4 macOS jobs.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

partial: each PR now starts 2 macOS jobs, and a docs-only PR skips the Go matrix and passes merge-ready, but the median wall time is 138 s, not ~90 s. macOS compute is bound by the runner's 3 cores (about 96-98 s however it is arranged, plus 25-35 s of checkout and cache restore), so 2 macOS jobs take about 100-155 s each.

Merged as 11968ca (PR #71).

- #71 (merged as 11968ca): two macOS jobs; gofmt, vet, the page renderer and pm-before on Linux alone; `go_changes.py` skips the Go jobs' steps on a PR that no Go job reads, and every check still reports; pm lint restores pm go's current cache; TestPullRacingLocalWriters drops two needless read-dedup waits (15 to 10 s under -race locally); the service's lock-free records read repeats under the records lock while git holds `index.lock`, which fixed a real flake (9 of 40 failures before, 0 of 40 after).
- #77: AGENTS.md says which PRs skip the Go jobs. #78: pm release build restores pm go's current cache. #79: internal/work's test seconds remeasured.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Median wall time to the last check ≤ ~90 s on 3 PRs: **not met.** It was 138 s, from #71 138 s, #77 88 s and #79 141 s; #71's other heads took 165 s and 107 s, and #79's rebase took 170 s. Before: 115 s idle median (PR 55) and 225 s with overlap. Other sessions' old-layout PRs tonight took 109, 202 and 228 s. The macOS jobs set the floor: build-test 101-160 s and race 82-133 s.
- At most 2 macOS jobs per PR: **met.** #71, #77 and #79 each started 2, against 3 before (4 with the release build). #78 started 3 because the path-filtered release build ran for its own change.
- A tests/*.py-only or docs-only PR skips the Go matrix and passes merge-ready: **met.** On #77 (AGENTS.md only), go_changes printed `go=false`, the 5 pm go jobs and lint-go/lint-deadcode finished in 4-11 s, the wall was 88 s, and `release/merge_ready.py 77` passed before the merge.
- No test or check dropped: **met.** Per OS, from the `go test -v` logs of #70 (main's layout) and #71: macOS 163 results (117 top-level) plus 32 internal/work tests plus 9 race tests and internal/service's 39 results; Linux the same counts. The sorted lists are identical. gofmt, vet and the renderer and pm-before builds run on Linux only; they read the same files on both OSes.
