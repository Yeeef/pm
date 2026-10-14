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

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
