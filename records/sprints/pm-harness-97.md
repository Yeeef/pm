---
type: sprint
title: Show work that sits in no sprint on the site
bead: yeeef-agents-9va.107
---

## Goal

> What should be true when this sprint ends, and why now?

The pm site lists every open item that sits in no sprint, so work filed straight under a project epic, or with no parent, is seen and gets planned. On 2026-10-08, seven open pm-harness items sat outside every sprint and showed nowhere on the site.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A "Not in a sprint" list on the site: each open, non-epic item whose parent is a project epic or that has no parent, with its id, type, title and project.
- Show it on the project page and on the overview.

**Out:**
- Moving the current unsprinted items into sprints.
- The `pm sprint open` crash on flat-id children of a project epic.
- Flagging these items in `pm show` or `pm check`.

## Done when

> What evidence will show the goal is met?

- A test with one task filed directly under a project epic and one with no parent shows both in the site's "Not in a sprint" list, and a task under a sprint does not appear there.
- On this repo's site, the list shows the items that sit outside every sprint at the time of the check.

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

- On 2026-10-09 the branch's overview listed 5 open items outside every
  sprint, all bugs/tasks filed straight under the pm-harness epic (.26, .54,
  .58, .59, .104); an independent bd query over 555 issues found the same 5
  and no parentless open item.

- The Go work store refuses an open item with no parent, so after the Go
  cut-over the parentless half of the list can only be empty; and progress()
  already draws a task filed directly under a project as a sprint box saying
  no tasks yet (left as is, out of scope).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: the site's overview and project pages list open work that sits in no sprint, in both the Python and the Go site, through PR #88.

Merged as 1bf3ee7 (PR #88).

- A "Not in a sprint" table on the overview and on each project page. It lists open non-epic items under a project epic, or with no parent (overview only).
- A need under a project is left out, because the await-you sections show it.
- The Go port keeps parity with Python on the fixtures, constructs and live corpora.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Done when | Met | Evidence |
|---|---|---|
| A test with a task under a project epic and a parentless task lists both; a sprint task does not appear | Met | `test_site_lists_open_work_that_sits_in_no_sprint`; `make test`: 110 passed, 136 skipped, 0 failed |
| On this repo's site, the list shows the items outside every sprint at the time of the check | Met | 2026-10-09: the branch's overview lists 5 items, all under the pm-harness epic. An independent `bd list` query over 555 issues found the same 5. `PM_PARITY_LIVE=1 make test-go` exits 0 |
