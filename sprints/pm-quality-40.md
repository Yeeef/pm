---
type: sprint
title: Harness date checks survive a run that crosses midnight UTC
bead: pm-d2k5.40
---

## Goal

> What should be true when this sprint ends, and why now?

The first CI run of PR #104 failed three harness tests only because the job crossed midnight UTC. `tests/test_pm.py` sets `TODAY = date.today()` once, at import, while pm stamps the date when it writes (feedback entries, decision blocks). Each such run costs a re-run and a misdiagnosis. When this sprint ends, no harness test depends on the wall clock staying on one date for the length of a run.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Find every harness test that compares a date or time pm wrote against one the test computed (`TODAY`, `date.today()`, `datetime.now()`, and the Go tests' equivalents).
- One mechanism that removes the class: either pm reads a fixed clock from an environment variable that only the harness sets, or each test reads the date pm wrote and checks its shape and its place in the output. Pick by what the code already has, and record the choice as a sprint decision.
- A test that would have caught this: a run with the clock set just before midnight.

**Out:**
- Time zones other than UTC in pm's output.
- Any user-facing clock option. If an environment variable is the mechanism, it is a test hook that `--help` does not document.

## Done when

> What evidence will show the goal is met?

- `grep` over `tests/` finds no date or time computed at import time and compared with pm's output; each remaining comparison reads pm's own date or runs under the fixed clock.
- The three tests that failed on PR #104 (`test_feedback_about_any_project_goes_to_the_repo_s_one_feedback_doc`, `test_sprint_edit_renames_the_sprint_in_both_stores_and_records_why`, `test_a_pr_that_changes_covered_code_waits_until_its_design_page_is_updated_or_waived`) pass with the clock moved past midnight between the test's setup and pm's write (a harness test shows it).
- `make test` passes, and the PR's CI is green.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-17}
The harness pins the date pm stamps with PM_TEST_TODAY, read in store.Today and set once per run by conftest; no help text names it, and unset it is the wall date.
store.Today already writes every date the tests compare, so one function and one harness line remove the class; tests reading the date back would need about 8 parsed assertions and still no cross-midnight proof.
:::

::: decision {source=agent date=2026-10-17}
The design page records-store stays as it is for this sprint's code changes.
store.Today gains a test-only date pin that no help text names; with it unset the date is the wall date, so the store's design is unchanged.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Seven assertions in tests/test_pm.py compared pm's stamped date with `TODAY`
  taken at import, and `TestDayActivityHoldsOnlyThatDaysEvents` dated its day
  from `time.Now()` while git stamped commits with its own clock. With
  `PM_TEST_TODAY` set to yesterday, the three tests that failed on PR #104
  pass; built without the fix, they fail (3 failed).

- CI found a fourth clock race of the same kind: the site printed `data-asof`
  with `%.3f`, which rounds, so data read at .2566 s showed as .257, after the
  test's write at .25699 s. `data-asof` is now cut to the millisecond; the
  golden test feeds a fraction that rounding would carry up.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
