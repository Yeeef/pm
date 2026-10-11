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

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
