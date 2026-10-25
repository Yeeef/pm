---
type: sprint
title: Overview lists open projects first, closed ones collapsed at the bottom
bead: pm-bfli.6
---

## Goal

> What should be true when this sprint ends, and why now?

The overview page lists every open project first, with its sprints, design pages and docs as now, and puts every
closed project in one collapsed section at the bottom. Today closed projects sit among open ones in record order, so
the owner scrolls past finished work to find what is live. Asked by the owner, from another project's feedback.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `RenderIndex` in `internal/site`: open projects in their current order, then a collapsed "Closed projects (N)"
  section holding the closed ones, each rendered as today.
- The site's stylesheet for the section; golden and unit tests.

**Out:**
- `pm show`'s project lines (a terminal list, nothing to collapse).
- The project pages themselves.

## Done when

> What evidence will show the goal is met?

A test with one open and one closed project renders the open project's heading before a collapsed
`<details>` section whose summary reads "Closed projects (1)" and which holds the closed project's heading; `make
test-go` and the PR's CI pass; a live check on a scratch service shows the same.

## Design pages

> Where is the detail?

- [Views and site](../design/views-and-site.md): the Root page row now says closed projects fold into one
  collapsed section at the bottom.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Live check: a scratch clone's pm service run served the overview as: Live
  one (open), then <details> 'Closed projects (2)' holding Demo and Old;
  Chromium showed the closed headings hidden before clicking the summary
  (is_visible False) and visible after (True).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: the overview lists the open projects first and folds the closed ones into one collapsed "Closed projects (N)"
section at the bottom.

- `RenderIndex` splits projects by their work-store status; a project record with no item counts as open.
- Two stylesheet rules give the section's summary the weight of a project heading, muted.
- The constructs golden overview, a new site test, the Views and site design page, a changelog entry.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Item | Met | Evidence |
|---|---|---|
| A test with an open and a closed project renders the open one before a collapsed "Closed projects" section that holds the closed one | yes | `TestTheOverviewListsClosedProjectsCollapsedAfterTheOpenOnes` adds a closed project that sorts before the open one and asserts the order open, `Closed projects (2)`, both closed headings, `</details>`; it fails on the old `RenderIndex` and passes on the new |
| `make test-go` and the PR's CI pass | yes locally | `make test-go` exit 0; `make test` 156 passed, 169 skipped; CI on the PR |
| A live check on a scratch service shows the same | yes | A scratch clone's `pm service run` (harness fixture: one project closed by seed, one opened by `pm project open`) served `/` with the open project's heading, then the section holding both closed ones; in Chromium the closed headings were hidden until the summary was clicked, then visible |
