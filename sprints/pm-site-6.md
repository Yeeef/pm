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

- Live check: a scratch clone's pm service run served the overview as: Live
  one (open), then <details> 'Closed projects (2)' holding Demo and Old;
  Chromium showed the closed headings hidden before clicking the summary
  (is_visible False) and visible after (True).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
