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

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
