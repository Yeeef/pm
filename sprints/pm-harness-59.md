---
type: sprint
title: Fast harness test suite
bead: yeeef-agents-9va.68
---

## Goal

> What should be true when this sprint ends, and why now?

`make test` gives fast feedback, because a full run now takes about 1.5 min and slows every change to the harness.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** measure per-test time in `skills/project-management/harness/tests/`; prune aggressively, keeping about 10% of the 275 tests (a soft target): only tests whose failure would mean a real break for an agent or the owner; speed up slow fixtures and tests (shared setup, fewer subprocess and git calls, parallel runs); split the suite into fast tests, run during development, and slow tests, run once a PR is ready for review, with a make target for each and the harness rules saying when to run which.
**Out:** the live tests (`make test-live`); changes to pm behavior made only to speed up tests.

## Done when

> What evidence will show the goal is met?

- A report lists the slowest tests and each removed test with its reason.
- The fast set (`make test`) runs in at most 10 s on the Mac, and the full set (fast plus slow) in at most 30 s, measured before and after.
- The harness rules say to run the fast set during development and the full set before raising a PR review.
- About 10% of the tests remain; the report states the rule used to keep a test and lists what each kept test protects.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-07}
Prune the harness tests aggressively, to about 10% of the 275 tests (a soft target), keeping only tests whose failure means a real break for an agent or the owner.
The owner asked to keep only truly useful tests; most tests pin details that cost run time and maintenance without catching real breaks.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Baseline on 8 cores at load ~3.5: 443 cases (275 functions) passed in 291 s;
  the slowest tests start pm serve or run pm setup (up to 7.2 s each).

- Pruned to 36 cases (32 functions, 12%): 42 s at load ~4.5; the rest pin
  wording, flags, refusal messages or repeat a kept path (commit 888c7f7).

- Remaining cost: each pm call runs uv run (~0.2-0.4 s), each pm serve start
  ~0.5-1 s plus a fixed 2 s wait in the merge test, and the repo fixture does
  git init per test.

- pytest-xdist (-n auto) cut the pruned suite from 37.8 s serial to 12.2 s;
  running pm.py without uv run saved ~18 ms per call, within noise, so it was
  reverted.

- Split: make test runs 22 fast cases in 5.6 s, make test-full runs all 36 in
  14.3 s (load ~6, 8 cores); the slow 14 start pm serve, a clone, a remote or
  the session-start hook.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: the harness tests were pruned from 275 to 32 test functions and split, so `make test` runs in about 6 s and `make test-full` in about 14 s, down from about 291 s.

Merged as b975883 (PR #61).

- Commit 888c7f7 keeps only end-to-end flows whose failure breaks an agent or the owner, and deletes `test_push.py` and dead fake switches.
- Commit b054348 runs the tests in parallel with pytest-xdist.
- Commit 9d2182e marks 14 slow tests, adds `make test-full`, and adds the rule to run it before a PR review.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Report of the slowest tests and the removed ones: met. The Findings hold the 291 s baseline and the slowest costs; the PR description lists the 20 slowest tests, the kept tests and the removed categories with counts.
- Fast set at most 10 s, full set at most 30 s: met. At load ~6 on 8 cores, `make test` ran 22 passed in 5.6 s and `make test-full` 36 passed in 14.3 s, against 443 cases in 291 s before.
- Rules say when to run each set: met. RULES.md, under Checks and delegation, says to run `make test` during development and `make test-full` before `pm action need --pr`.
- About 10% of the tests remain, with a keep rule and what each kept test protects: met. 32 of 275 functions (12%) remain; the PR description states the rule and one line per kept test.
