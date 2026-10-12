---
type: sprint
title: A PR's CI finishes in about 2 minutes
bead: pm-d2k5.18
---

## Goal

> What should be true when this sprint ends, and why now?

A PR's CI finishes in about 2 minutes instead of 3.2–4.8. Every PR's critical path is the darwin `work` or `race` job, and its time is `internal/work`'s 87 tests run serially (128 s on darwin, 82 s on linux, 27 s of CPU), plus 1 GB cache restores and a cold release-build compile.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Shard `internal/work`'s tests across processes in the `work` and `race` jobs of `pm-go.yml`, balanced by measured durations, and in `make test-go` locally.
- `test_release.py`'s builds reuse the real GOCACHE and GOMODCACHE.
- Trim the Go cache before it is saved on main.
- An unreachable remote fails a write fast instead of waiting out Dolt's fetch retries (9.6 s), with its test shortened to match.

**Out:**
- Dropping darwin from PRs (owner decision: keep it).
- Trimming tests (its own sprint).

## Done when

> What evidence will show the goal is met?

- Median wall time to the last check on 3 PRs after the change, from `gh run view --json jobs`, is at most about 120 s, against 191–290 s before.
- `go test ./internal/work` locally at 8 shards takes under 25 s, all passing.
- `TestCreateRefusesWithTheRemoteUnreachable` takes under 2 s.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-11}
Keep the darwin Go jobs on every PR
Sharding already brings a PR to ~2 min; darwin-only failures stay caught before merge
:::

::: decision {source=agent date=2026-10-11}
No production change for the unreachable-remote fast-fail: only the test changes, to hold the refusal a write past the read dedup gets.
A real write already refuses in ~5 ms; the 9 s path needs the remote lost within 1 s of the last read, and bounding Dolt push retries would reclassify slow legitimate pushes (PushTimeout 60 s) without changing the unknown-outcome semantics.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- TestCreateRefusesWithTheRemoteUnreachable's 9.6 s was Dolt's git-remote
  write retry (32 attempts, 5 ms doubling to a 320 ms cap) on the push's own
  failing fetch, reached only because the test created within Dolt's 1 s read
  dedup of its last remote read, so the pull read nothing. A service's write
  past that window refuses at the pull in about 5 ms ('a child id is minted
  only against the remote'; measured with the window waited out). So a real pm
  write against an unreachable remote does not wait 9 s, except one made
  within 1 s of the store's last remote read with the remote lost in between.
  The test now restarts A's service first: 9.6 s to 1.1-1.5 s.

- go vet and go test compiled every package a second time because only
  go-build used -trimpath: a cold GOCACHE after build+vet+test was 2455 MB in
  57 s (32 cores); with -trimpath on vet and test it is 1269 MB in 39 s. That
  doubling, not stale entries, is most of the ~930 MB compressed build cache:
  main's saves grew only 909 to 931 MB over three pushes. Aging the restored
  cache (mtime now-2h) and deleting what the run left untouched rebuilt cmd/pm
  afterwards with 0 compiles (12408 files, 7878 removed).

- internal/work sharded by measured seconds (LPT) in 8 processes: 12.5-12.6 s
  locally (3 runs, 87 tests, all pass) against about 100 s in one process;
  RACE_TESTS in 8 shards 11.2 s against 35 s. On CI's first run (macos-14, 3
  cores): 49.7 s for the work tests and 26.6 s for race, CPU-bound. The
  release build test with the real Go caches: 61 s to 25 s locally.

- PR 44's CI, 4 samples (wall from the first job created to the last done /
  longest job run time / longest macOS queue): 309/216/93 s, 497/152/377 s,
  373/196/248 s, 260/173/124 s; median wall 341 s against 191-290 s before.
  The jobs themselves got shorter (darwin work job 217-251 s before, 107-196 s
  now; its tests 50/37/65/45 s in 8 shards on 3 cores, against 155-192 s), but
  macOS runners queued up to 377 s behind other sessions' PRs running at the
  same time, and this PR restores main's cache, whose vet and test compiles
  lack -trimpath, so its build-vet-test and work jobs compile those cold
  (work's go test -c 48 s) until a push to main saves the trimmed cache. Under
  contention the two racing tests dominate a shard:
  TestCreateRacingALocalWriter 32.6 s and TestPullRacingLocalWriters 30.0 s on
  macos-14, against 4.4 and 9.7 s locally.

- After #48's trim, rebasing #44 remeasured internal/work's test seconds: 24
  tests, 8 shards, 27.2 s locally, bounded by one test
  (TestSyncTwoClonesConcurrentEditsMergePerTheTable, 26.8 s locally, 39.1 s on
  darwin-arm64 CI) that sharding cannot split. The folded shared-pair cases
  also exposed a read-dedup race (a create's remote read not recorded), fixed
  in #44 before merge.

- PR 53, the first PR after #44 merged, 3 samples (one run and two reruns of
  all four workflows, same head): wall from the first job created to the last
  check done 173, 148, 168 s, median 168 s, against 341 s on #44 and 191-290 s
  before the sprint; not yet about 120 s. Every Go job restored main's ffd2b26
  cache by its exact key, warm: go vet 0.6 s (linux) and 1.1 s (darwin), go
  test -c of internal/work 6-10 s on linux, 25-39 s on darwin (a link;
  darwin's cmd/pm link alone is 13 s; cold on #44 was 48 s). macOS queue was
  6-34 s, so queueing did not set the floor here. Per job, queue / run in s (3
  samples): work darwin 34/139, 9/139, 10/158; race darwin 8/146, 8/118,
  11/138; build-vet-test darwin 6/87, 7/87, 10/102; integration 3/71, 3/83,
  2/75; lint-deadcode 2/70, 2/95, 3/68; work linux 2/61, 3/65, 3/65;
  build-vet-test linux 2/61, 3/44, 3/49; light 3/58, 3/63, 3/62; race linux
  2/59, 3/59, 3/51; lint-go 3/39, 3/43, 3/40; lint-py and changelog 3-4/5-9.
  The floor is the darwin work job (139-158 s): setup-go 12-19 s and the 1 GB
  cache restore 34-37 s before any compile, go test -c 29-39 s, then the tests
  47-49 s, bounded by one test
  (TestSyncTwoClonesConcurrentEditsMergePerTheTable, 47-49 s alone in its
  shard on macos-14).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

partial: the Go jobs' own work is cut (internal/work's tests 100 s to 12.5 s locally, 37-65 s on macos-14; a cold Go cache halved), but the PR's measured wall time to its last check was not yet about 120 s: macOS runner queueing and main's not yet trimpath-warm cache dominated its runs.

- `make go-test-work` and `make -j2 go-test-race`: internal/work's tests in 8 processes balanced by measured seconds (`.github/scripts/go_shards.py`), failing unless every test ran and passed and the seconds file names every test.
- `go vet` and `go test` share go-build's compiles (`-trimpath`): cold GOCACHE 2455 to 1269 MB.
- A run that saves the Go cache trims it to what it used (`.github/scripts/go_cache_trim.py`).
- The release build test compiles with the real Go caches: 61 to 25 s locally.
- `TestCreateRefusesWithTheRemoteUnreachable` 9.6 to 1.1-1.5 s; no production change (sprint decision).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Item | Met | Evidence |
|---|---|---|
| Median wall to the last check on 3 PRs at most about 120 s | not met | PR 53, the first after the merge, 3 samples (a run and two reruns): 173, 148, 168 s, median 168 s, against 341 s on PR 44 and 191-290 s before. Go cache hit by exact key and warm (go vet 0.6-1.1 s; go test -c 6-10 s linux, 25-39 s darwin, against 48 s cold); macOS queue 6-34 s. The floor is the darwin work job, 139-158 s: setup-go and cache restore about 50 s, go test -c 29-39 s, tests 47-49 s bounded by TestSyncTwoClonesConcurrentEditsMergePerTheTable alone. PR 44's 4 samples: 309, 497, 373, 260 s |
| `go test ./internal/work` at 8 shards under 25 s, all passing | met | `make go-test-work`: 87 tests in 8 shards 12.5, 12.5, 12.6 s locally |
| `TestCreateRefusesWithTheRemoteUnreachable` under 2 s | met | 1.15 s in `go test -run`, 1.5 s with -v (3 runs) |
