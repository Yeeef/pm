---
type: sprint
title: "Agent lifecycle page: push reply delivery"
bead: yeeef-agents-9va.63
---

## Goal

> What should be true when this sprint ends, and why now?

The Agent lifecycle design page describes owner replies as sprint 40 delivered them, so agents and the owner read how replies reach a session today.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** updating `design/agent-lifecycle.md` (Asking the owner, its diagram, the hooks table, Alternatives considered, Open questions, and any other part sprint 40 changed) to the behaviour on main.

**Out:** changing harness behaviour.

## Done when

> What evidence will show the goal is met?

- Every statement about replies, waiting and hooks on the page matches the code on main, checked claim by claim.
- `pm render` passes and the diagrams render.

## Design pages

> Where is the detail?

- [Agent lifecycle](../design/agent-lifecycle.md)

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

Done: the Agent lifecycle page describes owner replies and PR merges pushed by `pm serve` into the asking session, as sprint 40 delivered them.

- `design/agent-lifecycle.md` updated (records commit 374bae8); the old waiter hook moved to Alternatives considered.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Statements about replies, waiting and hooks match main: met; each was checked against `pm.py`, `session_context_hook.py`, `beads.py`, `RULES.md` and both hook configs on main at f7403e8. The inbox's own behaviour (new turn when idle, read between tool calls when busy) comes from the Reply delivery page, not code, and a push into a busy session has not been seen live; the page says so.
- `pm render` passes and the diagrams render: met; 92 pages rendered, all 6 diagrams render with mermaid-cli.
