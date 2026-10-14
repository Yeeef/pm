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

- [Work store](../design/work-store.md): the question need kind, its ask fields and tables (schema 4), the parent rule, its closes and refusals (covers `internal/item/**`).
- [Owner-request Stop hook](../design/owner-request-hook.md): Ask-tool questions on the site, per runtime; the judge reading an open question as an open request.
- [Reply delivery](../design/reply-delivery.md): Questions: a Claude Code question answered only through its hook, a Codex one through `codex queue` after the rollout check, no idle dismissal of a Codex question.
- [Replies on the site](../design/site-replies.md): A session's question: the card, its inputs and the reply text rule.
- [pm in Codex](../design/pm-in-codex.md): the Codex Stop `pm hook ask` and its row in Areas.
- [Views and the site](../design/views-and-site.md): the "Sessions ask you" section.
- [Agent lifecycle](../design/agent-lifecycle.md): `pm hook ask` in the hook table per runtime, `pm hook inbox` closing a Codex question, the question need under Asking the owner.
- [pm CLI](../design/pm-cli.md): `pm hook ask` in Agent hooks, the questions' refusals in the answer commands and `pm need edit`, the question lines of `pm show`.
- [pm as an installable product](../design/pm-product.md): the `pm hook ask` entries, a Claude Code PermissionRequest and Codex's first Stop hook.
- [pm in Go](../design/pm-go.md): `hook ask` and the question's answer in the package table.
- [Sprint lifecycle](../design/sprint-lifecycle.md): an ask-tool question raised as a question need; the sprint page's "Sessions ask you" section.

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

::: decision {source=agent date=2026-10-13}
Supersedes this sprint's two decisions that made an ask-tool question a decision need: a question is its own kind of request. It asks the owner to steer one session (what to do next, confirm a scope, clarify intent); its answer is an instruction to that session, not a ruling, and lives as long as the session. A decision need asks for a ruling on a project's or sprint's outcome and becomes a recorded decision. A question whose answer turns out to rule the work is recorded by the agent as a decision with --confirmed.
Owner in chat: a decision is for a project or sprint outcome, not a session-wide instruction, e.g. "Do you want me to open a sprint for it?". Scope, not timing, sets them apart; timing follows from scope.
:::

::: decision {source=agent date=2026-10-14}
A question is a fourth need kind in the work store (question), with its call id and its questions and options in tables like review_targets: schema version 4.
The need_kind CHECK admits only decision, action and review, so the new kind changes the schema whatever else is stored; with the version moving anyway, tables keep the questions structured for the site's options and the no-JSON-cell rule. Cost: every clone upgrades before it syncs again, stated in the upgrade guide.
:::

::: decision {source=agent date=2026-10-14}
An open question's parent is the newest open task its session holds, else none; only a question may be an open need without a parent.
A question steers one session, which may hold no task (a chat with no sprint, as when the ask tool was first allowed); forcing a parent would bring back the sprint-or-task overhead the ask tool exists to avoid.
:::

::: decision {source=agent date=2026-10-14}
A Claude question's site answer reaches the session only through its PermissionRequest hook, never the messaging-socket push; a Codex question's goes through codex queue only after the rollout shows no user message since the question.
The hook is what closes the open dialog with the answer; a socket push would land as a separate user turn after the dialog is answered some other way. Codex has no dialog to close, so the rollout check is the first-wins guard.
:::

::: decision {source=agent date=2026-10-14}
A Codex question is never dismissed for idleness: it closes when answered on the site, or by the first user message after it in the rollout (the chat answer).
An idle Codex TUI waiting on the owner looks the same as an ended session, and codex queue delivers to a thread when it next loads, so a late site answer still reaches the session; a stale question costs one card the owner can dismiss.
:::

::: decision {source=agent date=2026-10-14}
The design page decision-need-layout stays as it is for this sprint's code changes.
needs.go only refuses a question need in pm need edit and the answer commands; the page holds a decision need's parts, layout and checks, which are unchanged
:::

::: decision {source=agent date=2026-10-14}
The design page record-layer stays as it is for this sprint's code changes.
records.go only adds Sessions ask you to the generated headings a sprint or day record may not hold; the page lists no such headings, and its review-card line stays true
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

- Claude Code 2.1.296, pty runs: a subagent cannot call AskUserQuestion (3
  cases: background, model-passed foreground,
  CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1; ToolSearch found no such tool), so
  the PermissionRequest hook only ever races a main-session dialog. There the
  hook and the dialog start within 1 ms; a hook sleeping 20 s does not delay
  the dialog; a hook timeout of 86400 is accepted (schema has no max). Hook
  stdin has no tool_use_id (session_id, transcript_path, prompt_id,
  permission_mode, tool_name, tool_input, ...). The tool_use and tool_result
  lines reach the transcript together about 5 ms after the dialog closes:
  toolUseResult.answers on an answer, is_error with toolDenialKind
  user-rejected on Esc. A hook failing or printing nothing leaves the dialog
  usable.

- make test-live after the question need joined the judge's open requests
  (judge prompt unchanged), 69 cases x 3 runs: Claude 206/207, median 2.62 s,
  p90 4.11 s; Codex 206/207, median 5.92 s, p90 13.98 s, max 20.01 s. The new
  case (an earlier turn's ask-tool question, open on the site, repeated in the
  reply: pass) 3/3 on both. Misses, 2/3 each, on cases this branch did not
  touch: Claude on 'formal-methods feedback 2026-10-10 03:31' (block), Codex
  on 'asks leave to push and open the PR' (needless).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
