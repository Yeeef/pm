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

done: this repo's pin moves from 0.8.6 to 0.11.0, which carries sprint 118's ask-tool questions on the site (0.10.0) and replies by push alone (0.11.0).

Merged as 70e5a81 (PR #96).

- `pm upgrade --to 0.11.0` wrote the pin and the hook entries: Claude Code's PermissionRequest and PostToolUse on AskUserQuestion, Codex's PostToolUse on request_user_input_async and UserPromptSubmit, `pm hook inbox` out of both.
- 0.11.0 installed on this machine; pins 0.5.0 to 0.8.5 that no checkout uses were pruned (593 MB).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- CI green and `make merge-ready` passes: met. [PR #96](https://github.com/Yeeef/pm/pull/96), 13 checks pass at 9eed4c4; `make merge-ready PR=96` prints its merge command.
- On the branch `pm version` reports 0.11.0: met. `pm doctor` there names only that the clone's service still runs 0.8.6, which holds until the move is on main and the main checkout pulls it; Codex's hook trust is the owner's step after that, raised on the site.
