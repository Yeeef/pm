---
type: sprint
title: Sort sprints and tasks by number, not as text
bead: yeeef-agents-9va.110
---

## Goal

> What should be true when this sprint ends, and why now?

The site and `pm show` list sprints and tasks in numeric id order, so Sprint 34 (.39) comes before Sprint 91 (.100). Today every list sorts Beads ids as text, so on the overview pm-harness reads .100–.108 before .39, out of sprint order.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- One natural sort key for Beads ids (the numeric parts compared as numbers), used wherever the site and `pm show` order items by id: the overview, the project page's Progress graph and sprint list, the sprint task lists and the Not in a sprint table.
- The same order in Python `site.py` and Go `internal/site`, so their pages still match.

**Out:**
- Ordering by status, priority or date.
- The selection of which done sprints are shown (the 8 latest closed).

## Done when

> What evidence will show the goal is met?

- A test with sprints .9, .39 and .100 under one project shows them in that order on the overview and the project page, and in `pm show`.
- `make test` and `make test-go` pass, and on this repo's overview pm-harness lists .39 before .100.

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

- Before the fix, pm-harness listed its 9 sprints with 3-digit ids (.100–.108,
  Sprint 91–98) before .39 (Sprint 34) on the overview and in pm show; from
  the branch, pm show lists .39 first and the rendered overview and project
  page put Sprint 34 before Sprint 91. Go already had a numeric compareIDs for
  ready lists; it is now exported and shared by the site.

- Before the fix, pm-harness listed its sprints with 3-digit ids (.100–.108,
  Sprint 91–98) before .39 (Sprint 34) on the overview and in pm show; from
  the branch, pm show lists .39 first, and the rendered overview and project
  page put Sprint 34 before Sprint 91. Go already had a numeric compareIDs for
  ready lists; it is now exported and shared by the site.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: the site and `pm show` list sprints and tasks in natural id order (.9 < .39 < .100), in both Python and Go, through PR #92.

Merged as ef4dad1 (PR #92).

- One order per language: Python `beads.id_key` and Go `work.CompareIDs`, the same total order.
- Used on the overview, the project page, sprint task lists, the Not in a sprint table, day pages and `pm show`.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Done when | Met | Evidence |
|---|---|---|
| A test with sprints .9, .39 and .100 under one project shows them in that order on the overview, the project page and in `pm show` | Met | `test_sprints_list_in_natural_id_order_on_the_overview_the_project_page_and_pm_show` passes, and fails with the change reverted |
| `make test` and `make test-go` pass; this repo's overview lists .39 before .100 | Met | `make test`: 110 passed, 136 skipped. `make test-go` and `PM_PARITY_LIVE=1 make test-go`: parity 116 passed, 1 skipped. From the branch, `pm show --project pm-harness` and the rendered overview list Sprint 34 (.39) before Sprint 91 (.100). PR CI: 5 of 5 checks pass |
