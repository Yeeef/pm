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
