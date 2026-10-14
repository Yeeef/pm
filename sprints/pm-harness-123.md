---
type: sprint
title: Move this repo's pm pin to 0.11.0
bead: yeeef-agents-9va.133
---

## Goal

> What should be true when this sprint ends, and why now?

This repo runs pm 0.11.0, which includes 0.10.0's ask-tool questions on the site (sprint 118), so pm's own sessions use what pm-harness built. Its pin is 0.8.6, five releases behind.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**

- `pm upgrade --to 0.11.0` in a worktree, with every crossed release's upgrade steps followed or named.
- The pin-move PR.

**Out:**

- Any code change to pm.

## Done when

> What evidence will show the goal is met?

- The PR's CI is green and `make merge-ready` passes; on the branch `pm version` reports 0.11.0 and `pm doctor` names nothing but the steps only the owner can do.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-15}
Renamed the sprint from "Move this repo's pm pin to 0.10.0" to "Move this repo's pm pin to 0.11.0": 0.11.0 (PR #90, replies by push alone) released while the 0.10.0 pin move waited for CI.
Moving straight to 0.11.0 includes sprint 118's 0.10.0 and saves a second pin move.
:::

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
