---
type: sprint
title: Release pm 0.5.0 and move this repo's pin to it
bead: pm-d2k5.14
---

## Goal

> What should be true when this sprint ends, and why now?

pm 0.5.0 is published with this wave's changes (option C, one feedback doc, pm clean, pm service stop, task close/move, sprint move, the Stop hooks, the site's diagrams and images, the Dolt fix), and this repo runs it, so the steps that wait on a release can run here: the feedback docs merged into one, and `pm clean --apply` on this clone.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- The release PR: `[Unreleased]` becomes `## [0.5.0] - <date>` with its summary and upgrade guide; tag `pm-v0.5.0` on the merge; the release workflow publishes it.
- Install 0.5.0 on this machine, `pm upgrade --to 0.5.0` in a PR, then the upgrade guide's remaining steps in this clone (service restart, untrack records/ if asked).

**Out:**
- Moving formal-methods, ai-safety and yeeef-agents to 0.5.0.

## Done when

> What evidence will show the goal is met?

- `gh release view pm-v0.5.0` lists the four assets, and `pm version` on the installed binary prints 0.5.0.
- This repo's `.pm/config.toml` pins 0.5.0 on main, `pm where` shows 0.5.0 and the service answers, and `pm doctor` reports nothing to fix.

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
