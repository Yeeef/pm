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
