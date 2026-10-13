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

::: decision {source=agent date=2026-10-13}
The Claude ask hook entry gets a 24 h timeout; the live check confirms Claude Code accepts it, and a smaller value is used if it does not.
The hook process idles cheaply, and a site answer can reach the dialog only while the hook runs; an owner away for hours must still be able to answer.
:::

::: decision {source=agent date=2026-10-13}
Codex first-wins is a rollout check right before codex queue; the window between check and queue is accepted, not locked.
The window is about a second, and a double answer costs one extra user turn the agent can see; closing it would need control of Codex we do not have.
:::

::: decision {source=agent date=2026-10-13}
The first task is a pty test of the subagent path, before any other code.
If Claude Code runs the PermissionRequest hook before the dialog for subagents, the hook must step aside there, and how it can tell decides the hook design.
:::

::: decision {source=agent date=2026-10-13}
An ask-tool question is a decision need tied to a live session (the session waits on it now), not a third need kind; once answered it closes as setting no rule unless the agent records a decision citing it.
Decision and action say what the owner gives; a question differs only in how the answer travels (to one waiting session, at once), so a new kind would duplicate the decision need on the site and in the rules.
:::

::: decision {source=agent date=2026-10-13}
A request to the owner has a kind (decision, action, review) and a timing: sync (a session waits on it now: light form, tied to that session, closed by the first answer in chat or on the site) or async (today's need). An ask-tool question is a sync request; the site marks a sync request as a session waiting now.
The owner framed it in chat: what sets a question apart is that the session waits for it now, not what the owner gives. Kind and timing are independent (a sync action exists too), so timing is a property, not a fourth kind.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Claude Code 2.1.296: a PermissionRequest hook (matcher AskUserQuestion) runs
  unawaited beside the open dialog and shares its claim(). Live pty run: the
  hook printed allow with updatedInput.answers {"Which color?":"Red"} 3 s
  after the dialog showed; the dialog closed with 'Allowed by
  PermissionRequest hook' and toolUseResult.answers held Red. Answered locally
  first (Blue), the hook was not killed and its late answer was ignored, so
  pm's hook must exit by itself. answers is keyed by question text.

- Codex 0.162.1, Default mode: request_user_input_async does not block; it
  writes an AgentMessage {delivery: async, questions: [...]} to the rollout
  and the turn ends; the answer is the next user message. codex queue --thread
  <id> --message Red got 'chose Red' in 3.4 s and 8.2 s (2 runs). Nothing
  enforces first-wins: pm must check the rollout for a user message after the
  question before queueing.

- Rejected paths (from Claude Code's strings, not run): the MCP channels
  permission relay skips requiresUserInteraction tools and carries only
  allow/deny; the Remote Control bridge races the dialog but needs the
  claude.ai bridge. Codex's blocking request_user_input is the app-server
  request item/tool/requestUserInput, reachable by a second client only with
  app-server --listen unix:// and TUI --remote; untested whether a pending
  request reaches that client.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
