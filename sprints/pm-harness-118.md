---
type: sprint
title: Ask-tool questions answerable on the site as well as in chat, first answer wins
bead: yeeef-agents-9va.128
---

## Goal

> What should be true when this sprint ends, and why now?

An agent's ask-tool question (Claude Code's AskUserQuestion, Codex's request_user_input_async) shows on the owner's site as a request the owner can answer there or in the session's chat; the first answer reaches the session and the other surface closes. Now: the Stop hook steers agents to the ask tool when the owner is in the chat, but the question is invisible on the site, so an owner who stepped away never sees it and the session waits with no trace.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**

- Claude Code: a PermissionRequest hook on AskUserQuestion (`pm init` writes it) that raises the question as a need, races the open dialog, and on a site reply returns allow with `updatedInput.answers`; it exits on its own when the dialog is answered in chat, closing the need with that answer.
- Codex: the async question (an AgentMessage with `delivery: async` in the rollout) raised as a need; a site reply goes through `codex queue` only when no user message followed the question; a chat answer closes the need.
- The site: the need shows its questions and options, and the reply form takes an option or free text.
- The design pages these touch (owner-request hook, reply delivery, pm in Codex) edited to the new state; harness tests with the fakes.

**Out:**

- Codex's blocking request_user_input (Plan mode only; needs the app-server on a socket).
- The subagent path where Claude Code runs hooks before the dialog instead of racing it.

## Done when

> What evidence will show the goal is met?

- A live check in a scratch clone, each runtime in a TUI under a pty: an ask-tool question shows on the site; answered on the site, the session continues with that answer and the need closes; answered in chat, the need closes with the chat answer and no site reply is delivered later.
- `make test` and the PR's CI are green, with harness tests for both races and both runtimes.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-13}
An ask-tool question is answerable both in the session chat and on the site; the first answer wins and the other surface closes.
Owner chose it in chat over a one-way mirror: an owner away from the terminal must be able to answer from the site without losing the in-chat path.
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
