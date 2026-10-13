---
type: sprint
title: "Per-session interactive mode: pm interactive / pm autopilot gate the owner-request judge"
bead: yeeef-agents-9va.129
---

## Goal

> What should be true when this sprint ends, and why now?

A session the owner is chatting in can turn off the owner-request Stop hook for itself with `pm interactive`, and turn it back on with `pm autopilot`. Other sessions are not affected. Today the judge blocks every ask made in chat, even when the owner is right there and wants to answer in chat. Each ask then has to go through AskUserQuestion or become a need.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm interactive` and `pm autopilot`: they write and remove a mode file under `.pm/run/sessions/<session-id>`, using the session id from the environment (`internal/session`). They refuse when no session id resolves.
- `pm hook owner-request` reads the hook input's `session_id` and passes the stop through when that session is interactive. Sessions start in autopilot.
- `pm hook stop` (the uncommitted-records check) stays on in both modes.
- Mode files of dead sessions: ignored by the hook and removed by `pm clean`.
- The rule in its three texts, edited in one commit: `prime.md`, the judge prompt and the block reasons. Labelled cases where the rule moves.
- Both runtimes: Claude Code and Codex.

**Out:**
- Any change to the uncommitted-records check.
- Syncing the mode across clones (it is local to the clone).

## Done when

> What evidence will show the goal is met?

- `test_owner_request_hook.py`: an interactive session's chat ask passes the stop; the same ask in another session, in an autopilot session, or after `pm autopilot` is blocked. A dead session's mode file is ignored.
- `test_pm.py`: `pm interactive` and `pm autopilot` write and remove the file, and refuse with no session id.
- Live check: a `claude -p` session runs `pm interactive`, asks in chat, and is not blocked; after `pm autopilot`, the same ask is blocked.
- PR on main.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-13}
Interactive mode turns off only the owner-request judge; pm hook stop (uncommitted records) stays on in both modes.
An uncommitted hand edit is a mistake whether or not the owner is watching; only the chat-ask rule depends on the owner being present.
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
