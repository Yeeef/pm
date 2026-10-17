---
type: sprint
title: pm init installs the pi extension
bead: pm-puvw.2
---

## Goal

> What should be true when this sprint ends, and why now?

`pm init` installs a pi extension that bridges pi's in-process events to pm's hook commands, so a pi session gets the same session-start context and turn-end checks as Claude Code and Codex. pi has no shell hooks, so the extension is the hook layer.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A pm extension (TypeScript, embedded in the release) written to `.pi/extensions/`, with pi's project trust handled.
- `session_start` and `before_agent_start` run `pm prime` and inject its output; `tool_call` runs pm's pre-tool hooks and blocks with their reason; `agent_before_settle` runs the Stop hook and, on a block, appends the reason and continues, with a loop guard.
- `pm doctor` and `pm uninstall` know the extension; the hook parity test compares three runtimes, with each pi difference given a reason.

**Out:**
- The ask tool, transcript reading of pi sessions, the judge on pi.

## Done when

> What evidence will show the goal is met?

- `make test` and the install golden tests pass; `TestClaudeAndCodexRunTheSameHooks` covers pi.
- Live check in a scratch clone: `pm init` then a pi session gets `pm prime`'s context; leaving a record uncommitted makes the turn continue with the Stop reason; `pm uninstall` removes the extension.

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
