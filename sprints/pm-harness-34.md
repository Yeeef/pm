---
type: sprint
title: Codex integration for pm
bead: yeeef-agents-9va.39
---

## Goal

> What should be true when this sprint ends, and why now?

An agent running in Codex works on the pm harness with the same guarantees as one in Claude Code: it gets the session context, waits for owner replies, cannot leave records uncommitted or owner requests only in chat, and commits records from inside Codex's sandbox.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** a gap list of pm behaviour in Codex against Claude Code; closing the gaps in hooks, setup and docs; a live check from a real Codex session.
**Out:** packaging pm for other repos (the bundled-product sprint), beyond keeping the Codex pieces inside that layer.

## Done when

> What evidence will show the goal is met?

- A gap list of Codex against Claude Code is written, each gap closed or recorded as a decision to leave it.
- A real Codex session raises a need, is woken by the owner's reply, and commits a record, with the transcript as evidence.
- The setup docs say what a Codex user runs, and nothing else is needed.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-10}
Moved yeeef-agents-9va.39.1 to pm-v498.1: The owner moved pm's Codex integration work to its own project, pm-codex, on 2026-10-10.
This sprint's open work continues in pm-codex sprint 1.
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
