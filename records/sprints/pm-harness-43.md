---
type: sprint
title: Require Python 3.12 for pm
bead: yeeef-agents-9va.49
---

## Goal

> What should be true when this sprint ends, and why now?

pm runs again on machines where uv has a managed Python 3.11: the scheduled push, `pm show` and every write stopped at 02:27 UTC on 2026-10-06 when uv installed 3.11.11 and picked it over the system 3.12, because pm.py declares `>=3.11` but uses 3.12-only f-string syntax.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** bump `requires-python` to `>=3.12` in pm.py and the harness test runner.

**Out:** rewriting the code for 3.11.

## Done when

> What evidence will show the goal is met?

- `uv run pm.py` resolves a Python >= 3.12 and `pm show` runs with only managed 3.11 present.
- The harness tests pass.
- The scheduled push succeeds again after the PR merges.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- uv picks its own managed Pythons before the system one: the CPython 3.11.11 it installed at 02:25:38 UTC on 2026-10-06 replaced the system 3.12.3 for `uv run pm.py`, and the scheduled push failed 79 times from 02:27 UTC until it was found. Under 3.11, only pm.py line 316 fails to compile among the harness scripts.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: pm declares `requires-python >=3.12`, so uv no longer runs it on a managed 3.11.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- `pm show` runs with only managed 3.11 present: met; `bin/pm show` from the PR branch, with `cpython-3.11.11` still installed, prints project state.
- Harness tests pass: met; `uv run skills/project-management/harness/tests/run.py`: 376 passed, 7 skipped.
- Scheduled push succeeds after merge: met; after PR #47 merged as a692ee8 and the main checkout fast-forwarded, `.git/pm-push.log` shows `2026-10-06T15:42:40 beads ok` and `15:42:50 records ok: pushed 13 commit(s)`; `.records` is level with `origin/records`.
