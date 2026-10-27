---
type: sprint
title: A question skipped in the Claude app closes at the turn's end
bead: yeeef-agents-9va.140
---

## Goal

> What should be true when this sprint ends, and why now?

A Claude Code question the owner skips in the Claude app (Remote Control) leaves "Sessions ask you" once the session's turn ends, instead of staying open until the session's process exits (18 h in one case). The owner reported it. Through Remote Control, a skip returns "Denied by user" to the agent, but Claude Code sends the waiting `pm hook ask` no signal and fires no PostToolUse, so nothing closes the need.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Claude Code's Stop hook runs `pm hook ask`, which closes the session's open questions as dismissed: a Claude Code dialog never outlives its turn.
- The runtime-difference reason, the hooks' doc comments and the owner-questions design page.

**Out:**
- Codex: its questions outlive the turn by design and close at the next prompt.
- A skip that Claude Code never records (the dialog still open in the session): that question is still open.

## Done when

> What evidence will show the goal is met?

- A Go test: a Stop event closes the session's open question as dismissed and leaves another session's open; `make test-go` for `internal/hooks` and `internal/install` passes.
- Live: the built pm run on a Stop event for this session closes the open `[TEST] skip probe` question, and its waiting `pm hook ask` process exits.

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
