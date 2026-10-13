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

None yet.

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
