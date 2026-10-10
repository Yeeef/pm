---
type: sprint
title: Agent lifecycle design page
bead: yeeef-agents-9va.55
---

## Goal

> What should be true when this sprint ends, and why now?

An agent or the owner can see, on one page with diagrams, what an agent session goes through in a repo that uses the harness: what starts it, what is in its context, how it claims and does a task, how it asks the owner for decisions and actions and gets replies, and how it hands back.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** a new design page `design/agent-lifecycle.md`, linked from the pm-harness main design page, describing the current harness as it is (hooks, `pm` commands, Beads, records), with mermaid diagrams.

**Out:** changing any harness behavior; the sprint flow itself, which stays on the Sprint lifecycle page.

## Done when

> What evidence will show the goal is met?

- `design/agent-lifecycle.md` exists with the design-page sections, covers session start, context, claiming, working, needs and replies, subagents, and session end, with at least one diagram per major stage, each followed by a one-line reading.
- Every command, hook and file it names exists in the repo as described.
- The pm-harness design page links it, and `pm render` passes.

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

Done: the Agent lifecycle design page shows, with six diagrams, how an agent session starts, what its context holds, and how it claims, works, asks the owner and hands back.

- `design/agent-lifecycle.md`, linked from the Project management harness page (records commit 0248ab6).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Page with the design sections covering start, context, claim, work, needs and replies, subagents and hand-back, a diagram per major stage each followed by a reading: met; six mermaid diagrams, all rendered without error by mermaid-cli.
- Every command, hook and file named exists as described: met; each claim was checked against `pm.py`, the hook scripts, `.claude/settings.json` and `.codex/hooks.json`, except three taken from docs (SessionStart refiring after compaction, Codex not expanding `@` imports, subagents sharing the session id).
- Linked from the pm-harness design page and `pm render` passes: met; "rendered 84 pages", no errors.
