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
- The owner's prompt `pm interactive` or `pm autopilot` (the whole prompt): `pm hook inbox` on UserPromptSubmit writes or removes a mode file under the main checkout's `.pm/run/sessions/<session-id>`, using the hook input's `session_id`. No CLI command changes the mode, so a blocked agent cannot switch the judge off itself.
- `pm hook owner-request` reads the hook input's `session_id` and passes the stop through when that session is interactive. Sessions start in autopilot.
- `pm hook stop` (the uncommitted-records check) stays on in both modes.
- Mode files of sessions that are not live: removed by `pm clean`.
- The rule in its three texts, edited in one commit: `prime.md`, the judge prompt and the block reasons. Labelled cases where the rule moves.
- Both runtimes: Claude Code and Codex.

**Out:**
- Any change to the uncommitted-records check.
- Syncing the mode across clones (it is local to the clone).

## Done when

> What evidence will show the goal is met?

- `test_owner_request_hook.py`: an interactive session's chat ask passes the stop; the same ask in another session, in an autopilot session, or after `pm autopilot` is blocked.
- `test_hooks.py`: the prompt `pm interactive` / `pm autopilot` on UserPromptSubmit writes / removes the session's mode file, on both runtimes' input; any other prompt, one that merely contains the words included, changes nothing.
- `test_clean.py`: `pm clean` removes a dead session's mode file and keeps a live one's.
- Live check, in a scratch clone: a `claude -p` session given the prompt `pm interactive`, then resumed with a prompt that makes it ask in chat, is not blocked; after the prompt `pm autopilot`, the same ask is blocked.
- PR on main.

## Design pages

> Where is the detail?

- [Owner-request Stop hook](../design/owner-request-hook.md): Interactive mode, its prompts, mode file and checks; the UserPromptSubmit block measured on both runtimes; the mode read from the transcript, a `pm interactive` command and an unblocked mode prompt as alternatives not chosen (covers `internal/session/mode*.go`).
- [Agent lifecycle](../design/agent-lifecycle.md): the UserPromptSubmit row of the hooks table, and the Stop check off in interactive mode (covers `prime.md`, `internal/hooks/prime*.go`, `internal/hooks/stop*.go`).
- [pm CLI](../design/pm-cli.md): the same in its hooks table (covers `internal/cli/commands.go`, its help golden).
- [Session claims](../design/session-claims.md): `pm clean` removing the mode file of a session not live (covers `internal/cli/clean.go`, `internal/session/**`).

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-13}
Interactive mode turns off only the owner-request judge; pm hook stop (uncommitted records) stays on in both modes.
An uncommitted hand edit is a mistake whether or not the owner is watching; only the chat-ask rule depends on the owner being present.
:::

::: decision {source=owner date=2026-10-13}
The mode is set per session by pm interactive and pm autopilot, stored local to the clone, and defaults to autopilot.
Hooks are configured per project, so the hook itself must read a per-session flag; a missing or stale flag must fall back to the strict default.
:::

::: decision {source=owner date=2026-10-14}
Only the owner sets the mode: a prompt that is exactly pm interactive or pm autopilot, caught by the UserPromptSubmit hook. pm has no CLI command that changes the mode.
A shell command would let a blocked agent switch the judge off itself; only the owner can submit a prompt, so the UserPromptSubmit hook cannot be triggered by an agent.
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
