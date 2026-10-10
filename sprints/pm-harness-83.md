---
type: sprint
title: "Go port: implementation-neutral tests (P1)"
bead: yeeef-agents-9va.92
---

## Goal

> What should be true when this sprint ends, and why now?

pm's pytest suite runs against either implementation, so every later port sprint proves parity with the same 100 tests and a differential transcript instead of new tests. It comes first, beside the Go skeleton, because the records-and-site sprint and every parity run read work data through it.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `PM_IMPL=python|go` picks the pm binary the tests run.
- A store-neutral layer: tests seed and read work data as items; for Python it maps fake-bd JSON to `pm export` items (`repo.items()` replaces the `repo.issues()` and `bd_calls()` reads).
- A recorder in `Repo.pm` that logs every `pm` call: argv, stdin, stdout, stderr, exit code, changed record files and the store export.

**Out:** any Go code; the Go side of `repo.items()` (`pm export`, built by the work store core sprint); CI's expected-failure job for Go.

## Done when

> What evidence will show the goal is met?

- All 100 tests pass on Python with `PM_IMPL=python`, with no assertion weakened.
- A suite run writes one transcript per test.
- The sprint's PR is on main.

## Design pages

> Where is the detail?

- [pm in Go](../design/pm-go.md): Tests, Port order and coexistence

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-08}
Moved yeeef-agents-9va.92.3 to yeeef-agents-9va.94: Running the shared suite on Go pm needs both the neutral tests (sprint 83) and the Go skeleton (sprint 84) on main.
Sprint 85 is the first sprint that depends on both, so the task lands there and sprint 83 can close when its PR merges.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- The work-store import mapping, written as the tests' Python mapper
  (pm/tests/work_items.py), maps 516 of this repo's 517 bd issues (252 tasks,
  164 needs, 96 sprints, 5 projects; 255 comments). The one it refuses is the
  open parentless bug the design page names; the mapping table needs no new
  row for real data.

- Python pm shows an in_progress issue without metadata.claimed_by as 'held
  without a session'; the work store's holder is always a session and the
  import table has no row for it. Real data has 0 such items of 517; two test
  seeds (add_task in test_pm.py and the day-summarize seed) have one, so the
  tests' mapper gives them a holder with session null, and the Go side must
  choose how such a seed imports.

- Transcripts: of 105 light tests, 44 make at least one repo.pm call; the rest
  run pm through a direct subprocess (hooks, launcher, claims, stdin cases) or
  in process, so their transcripts are empty. Four light runs and two runs of
  11 integration tests gave byte-identical transcripts after normalising
  paths, mkdtemp names, commit ids (1 in 27 short ids is all digits, so the id
  pattern takes digit-only runs too), UUIDs, timestamps, durations, ports and
  minted root ids.

- 28 test functions (33 collected items) are Python-only: 10 in-process
  service tests, 5 hooks, 4 launcher, 2 config, 2 test_pm renders, test_tool's
  4 and test_migrate's 1. With PM_IMPL=go they skip with their reason; the
  remaining tests run the binary.

- Correction to the Python-only count above: the 28 marked test functions are
  30 collected items, not 33 (pytest --co -m impl: 30/176).

- A fresh-context review found the first item layer weaker than the bd
  assertions it replaced: changes() == {} missed a refused claim that
  re-stamps claimed_at, and the normaliser made a 7- and 40-char id, two time
  formats and '3m'/'3 min' equal. Fixed in eca9625: repo.unchanged() compares
  items with stamps plus (Python) no bd write; the normaliser keeps shape (ids
  numbered with length, time digits as 0, duration units). Left for the Go
  side: two answered-need tests keep bd's 'Response: <text>' form only under
  IMPL == python; the fake bd stores no external_ref, so the mapper cannot
  require it on reviews.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: pm's tests run against either implementation, read work data as items, and write one transcript per test.

Merged as 3806ce2 (PR #79).

- `PM_IMPL=python|go` picks the pm under test; Python-only tests carry an `impl` mark with a reason.
- `repo.items()` maps fake-bd issues to work-store items by the import table; `repo.changes()` and `repo.unchanged()` replace the bd-argv checks.
- `Repo.pm` records argv, stdin, stdout, stderr, exit code, changed records and the store export, one transcript per test.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Met:** the suite passes on Python with `PM_IMPL=python` in CI: 105 light tests passed (35 skipped: the live eval) and 36 integration tests passed, with no assertion weakened.
- **Met:** a light run writes one transcript per test, 105 in all; repeated runs give byte-identical transcripts.
- **Met:** the PR, [#79](https://github.com/Yeeef/yeeef-agents/pull/79), is on main as `3806ce2`.
