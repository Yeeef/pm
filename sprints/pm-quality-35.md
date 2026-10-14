---
type: sprint
title: The code and test metrics doc measures current main
bead: pm-d2k5.35
---

## Goal

> What should be true when this sprint ends, and why now?

The pm code and test metrics doc describes current origin/main: every table, figure and reading in it is measured at one commit of today's main. It last measured PR #53 (2026-10-12); about 35 PRs have merged since, the CI cuts of sprints 30 to 32 among them (median CI wall per PR 2.9 min then, 1.7 min now), so its numbers no longer say what pm looks like.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** rerun every step of `pm-quality-metrics.py` (size, complexity, lint, tests, runtime, coverage, ci, trend) at one origin/main commit; redraw the figures; rewrite the doc's header, tables and readings to the new numbers; extend the trend with the pm-quality sprints closed since.

**Out:** new metrics, or a change to the method beyond what the generator needs to run on today's tree; acting on what the numbers show.

## Done when

> What evidence will show the goal is met?

- The doc's "Commit measured" names an origin/main commit of 2026-10-14 or later, and every table and figure comes from that run's data.
- `pm commit` of the doc and its figures passes, and the site page https://pm.yeeefs.com/docs/2026-10-12-pm-quality-metrics.html shows the new commit and CI numbers.

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
