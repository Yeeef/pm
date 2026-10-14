---
type: sprint
title: A PR runs only the Go tests its change can affect
bead: pm-d2k5.32
---

## Goal

> What should be true when this sprint ends, and why now?

A PR runs only the Go test packages that its changed files can affect, and the full suite still runs on every push to main. Most per-PR Go cost is internal/work's tests, and today a change to site, records or install code runs them anyway. Selection must be exact (the import graph plus each package's non-Go inputs), never a guess, and must fail toward running more. Starts after sprint 31 merges, since both edit pm-go.yml and the Makefile.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A selector built on `go list -deps -test`: the reverse import graph from the changed `.go` files to the test packages that import them, test-only imports included.
- Non-Go inputs mapped to their readers: testdata, embedded files (`prime.md`, `style.css`, `prompts/`, `CHANGELOG.md`, `changelog.d/`), `go.mod`/`go.sum`, and helpers such as `tests/fake_sched.py`. Reuse and extend sprint 27's `go_changes.py`.
- A changed file the selector cannot place runs everything.
- The pm go and race jobs on both OSes run the selection.
- Sprint 31's macOS OS-dependent subset composes with it: the selection intersected with that subset.
- The full suite on every push to main stays.

**Out:**
- The Python harness: it drives the built binary, so any Go change runs it whole.
- `go test`'s result cache: it is not safe, because tests read git and other external state.

## Done when

> What evidence will show the goal is met?

- On 3 real PRs that touch only non-store packages, internal/work's tests do not run, and the jobs' compute and wall time are reported against the same jobs on main.
- A PR that changes internal/work runs the full set.
- A test case shows that a testdata or embedded-file change selects the package that reads it.
- A test case shows that an unplaceable file runs everything.
- `merge_ready.py`'s per-PR workflow rule still holds.
- One main-push full run is green.

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
