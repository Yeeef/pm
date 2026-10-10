---
type: sprint
title: Cap done sprints in the overview Sprints list
bead: yeeef-agents-9va.89
---

## Goal

> What should be true when this sprint ends, and why now?

The site overview's per-project Sprints list shows every non-done sprint and only the 8 most recently closed done sprints, so pm-harness's list stays readable.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the Sprints list under each project on the overview page; a note linking the project page for older done sprints; a test.
**Out:** the project page's Progress table and graph; new pages.

## Done when

> What evidence will show the goal is met?

- The pm test suite passes, with a test that asserts the cap of 8 done sprints and that every non-done sprint appears.
- The rendered overview for pm-harness shows all non-done sprints plus 8 done ones.

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

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: the overview's per-project Sprints list shows every non-done sprint and only the 8 latest closed done ones, in [PR #75](https://github.com/Yeeef/yeeef-agents/pull/75).

Merged as 7bb4dff (PR #75).

- `DONE_SPRINTS_SHOWN = 8` in the site; done sprints ranked by `closed_at`, newest first.
- A last line "N older done sprints not shown; the project page lists every sprint" links the project page, whose Progress table is unchanged.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Item | Met | Evidence |
|---|---|---|
| Test suite passes with a cap test | met | `make test`: 103 passed, 35 skipped; `test_index_lists_every_sprint_not_done_and_only_the_latest_closed_done_ones` (12 done, 3 live: all 3 live and done 04 to 11 shown, "4 older" note); PR CI light and integration green |
| pm-harness overview shows non-done plus 8 done | met | `uv run --project pm python pm/tests/render_pages.py` on this repo: pm-harness 78 rows before, 19 after (11 non-done, 8 done) plus "59 older done sprints not shown" |
