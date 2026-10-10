---
type: sprint
title: CI fails on Go code that gofmt would change
bead: pm-d2k5.17
---

## Goal

> What should be true when this sprint ends, and why now?

No unformatted Go reaches main: CI fails when `gofmt -l` lists any file. From pm feedback 2026-10-10 (pm-quality sprint 16 found `internal/cli/cli.go` on main not gofmt-clean, and no CI step checks formatting).

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A `make go-fmt` target that fails and names each file `gofmt -l` lists, run by `make test-go` and by pm-go.yml's build-vet-test job.
- Format the files main has today, in the same PR.

**Out:**
- Other linters.

## Done when

> What evidence will show the goal is met?

- `make go-fmt` fails on a branch with one unformatted file (shown in the PR) and passes on the PR's head; the PR's CI passes and `make merge-ready` says ready.

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
