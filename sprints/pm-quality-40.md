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
