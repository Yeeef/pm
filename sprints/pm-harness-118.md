---
type: sprint
title: Ask-tool questions answerable on the site as well as in chat, first answer wins
bead: yeeef-agents-9va.128
---

## Goal

> What should be true when this sprint ends, and why now?

An agent's ask-tool question (Claude Code's AskUserQuestion, Codex's request_user_input_async) shows on the owner's site as a request the owner can answer there or in the session's chat; the first answer reaches the session and the other surface closes. pm's rules also tell agents which kind of request they are making, so they choose the right tool: a question steers this session and goes through the ask tool; a decision rules a project's or sprint's outcome and is a decision need; a step only the owner can do is an action need. Now: the Stop hook steers agents to the ask tool when the owner is in the chat, but the question is invisible on the site, so an owner who stepped away never sees it and the session waits with no trace.

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
- The request rule in its three texts (`prime.md`, the judge prompt, the block reasons) says which tool fits which kind, with labelled judge cases for the cases it moves; `make test-live` gives every case's verdict on every run on both runtimes.
- `make test` and the PR's CI are green, with harness tests for both races and both runtimes.

## Design pages

> Where is the detail?

- [Owner questions](../design/owner-questions.md): the question kind end to end: what it is, its record, raising per runtime (Claude Code's PermissionRequest, Codex's PostToolUse), answering on the site and in chat, first answer wins, closing, the judge; a sequence diagram per runtime.
- [Work store](../design/work-store.md): the question need kind, its ask fields and tables (schema 4), the parent rule, its closes and refusals (covers `internal/item/**`).
- [Owner-request Stop hook](../design/owner-request-hook.md): the judge reading an open question as an open request; Ask-tool questions on the site, a summary linking Owner questions.
- [Reply delivery](../design/reply-delivery.md): Routes by request kind and runtime: a Claude Code question answered only through its hook, waiting on the service's wait socket; a Codex one closed by the service, then queued with `codex queue`; the sweep settling questions; the accepted windows.
- [Replies on the site](../design/site-replies.md): A session's question: the card, its inputs and the reply text rule; the wait socket under Security.
- [pm in Codex](../design/pm-in-codex.md): the Codex `pm hook ask` at PostToolUse (raise) and UserPromptSubmit (chat answer) and its row in Areas.
- [Views and the site](../design/views-and-site.md): the "Sessions ask you" section; the wait socket fed by the snapshot.
- [Agent lifecycle](../design/agent-lifecycle.md): `pm hook ask` in the hook table per runtime (raise, chat-answer close, SIGTERM dismiss), the form by what is asked and the question need under Asking the owner.
- [pm CLI](../design/pm-cli.md): `pm hook ask` in Agent hooks, the questions' refusals in the answer commands and `pm need edit`, the question lines of `pm show`.
- [pm as an installable product](../design/pm-product.md): the `pm hook ask` entries: Claude Code's PermissionRequest and PostToolUse, Codex's PostToolUse and UserPromptSubmit.
- [pm in Go](../design/pm-go.md): `hook ask`, the wait socket and the question's answer in the package table.
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

::: decision {source=agent date=2026-10-14}
The design page session-claims stays as it is for this sprint's code changes.
the branch adds question needs (their store rows, a pm show line for parentless questions, the read of a Codex chat answer after an ask-tool call); session resolution, claims, liveness and the holder display are unchanged
:::

::: decision {source=agent date=2026-10-14}
The Claude hook's window between its last transcript check and its store close is accepted: if the dialog is answered inside it, the need records the site answer while the session took the chat one.
The window is one store write (tens to hundreds of ms); closing it would need the hook to retract an answer already printed, which Claude Code offers no way to do. The session's own transcript stays the truth of what it was told.
:::

::: decision {source=agent date=2026-10-14}
The Codex ask hook runs on PostToolUse of request_user_input_async, taking the call id from tool_use_id, instead of at Stop reading the rollout for the turn's calls.
PostToolUse fires on the call itself with its id, so the question is raised the moment it is asked, whether or not the turn goes on, with no rollout scan to find it; Stop was chosen before the experiment showed Codex runs tool hooks for the ask tools.
:::

::: decision {source=agent date=2026-10-14}
A Codex question answered in chat is closed by the service alone (its rollout check at sweep and before any queue), not by pm hook inbox.
PR #90 (pm-quality sprint 34) removes pm hook inbox; the service already reads the rollout before it queues a site answer, so first-wins holds without the inbox hook, and this PR stops depending on which of the two merges first.
:::

::: decision {source=owner date=2026-10-14}
Scope adds: pm's rules tell agents which tool fits by what they ask, not by whether the owner is in the chat. A question (steer this session: what to do next, confirm a scope, clarify intent) goes through the ask tool; a decision (a ruling on a project or sprint outcome that later sessions follow) is a decision need; a step only the owner can do is an action need.
Owner in chat, 2026-10-14: the goal must make clear to pm agents what a decision is and what a question is, so they choose the right tool; now that a question also shows on the site, the owner being in the chat no longer decides it.
:::

::: decision {source=agent date=2026-10-14}
The Claude ask hook waits for a site answer on one blocking call to the pm service, which answers it when the reply is stored; the hook still tails the transcript itself for a dialog answer.
Claude Code takes a PermissionRequest answer only from the hook process's stdout, so the process must live while the dialog is open; but its 1 s poll dialed the service, handshook and read the need about 86,400 times a day per open question, while the service already learns of a reply the moment it is stored.
:::

::: decision {source=agent date=2026-10-14}
No transcript or rollout read for questions: a Claude chat answer closes the question from a PostToolUse hook on AskUserQuestion; Esc or a stopped session closes it as dismissed from the SIGTERM Claude Code sends the waiting hook; a Codex chat answer closes it from a UserPromptSubmit hook with the typed prompt; the service closes a question before it queues a site answer, so the close decides which answer wins.
Project rule (2026-10-14): a transcript is read only when no event gives the fact; the experiments show an event for every ending (PostToolUse, SIGTERM, UserPromptSubmit).
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

- Live check (Done when), parity eval on HEAD 16214e0, Claude Code 2.1.296 and
  Codex 0.162.1 TUIs in pty: PM_PARITY_CODEX_SANDBOX=danger-full-access
  PM_LIVE_TESTS=1 uv run pytest -n 4 tests/test_agent_parity_live.py -k
  ask_tool: 4 passed in 91 s. Answered on the site: Claude's dialog closed
  with toolUseResult.answers Blue from the hook, 1.0 s after the owner's POST;
  Codex continued on the queued answer in 1.5 to 9.5 s; the question closed
  answered on the site, delivered 1. Answered in chat: the question closed
  with the chat answer, delivered 0, and a later POST from the stale card was
  refused 400 already closed. Whole parity set: 14 passed in 150 s (with the
  default sandbox, 3 Codex scenarios fail on this host's bwrap, as
  documented).

- Codex 0.162.1: the TUI's pick list (shift+left) for an async question is
  offered only while the turn runs; pm raises the question at Stop, and once
  the turn is idle the owner answers in chat by typing, not picking.

- Review (fresh context) found two bugs, fixed with tests that fail without
  them: a question with a newline, double space or edge whitespace got the
  wrong site answer (site and hook normalised the text differently), and pm
  hook inbox read the session's needs twice, so a site reply landing between
  the reads was closed as delivered without reaching the session. Migration
  2->4 and a v3/v4 two-clone merge were checked in a scratch copy and hold.

- Codex 0.162.1, TUI under a pty: PreToolUse and PostToolUse fire for
  request_user_input_async (Default) and request_user_input (Plan);
  PermissionRequest fires for neither. Hook stdin carries tool_use_id (the
  rollout's call_id), turn_id, transcript_path, tool_input. PostToolUse's
  tool_response is {accepted:true} for the async tool and
  {answers:{<id>:{answers:[...]}}} for the blocking one. A PreToolUse deny
  with a reason reaches the model as the tool output (Default mode, observed),
  so it can carry an answer; Plan mode untested.

- Request rule by kind (9467a0f): prime.md, the block reason and the judge
  prompt pick the form by what is asked (question: ask tool, or a decision
  need with no ask tool; decision: decision need; action: action need); the
  presence test is gone. 3 judge cases added, none moved. make test-live, 78
  cases x 3 runs: two clean runs, Claude 234/234 (median 2.4 s, p90 4.2 s),
  Codex 234/234 (median 5.4 s, p90 13.8 s). Three earlier runs each had a miss
  (one Claude timeout; Codex blocking 'Should I push the branch and open the
  PR?' in 1 of 3 runs, fixed in the prompt by excluding leave for an
  authorized step from the decision kind); the Codex miss rate before the fix
  was about 1 in 6 runs, so two clean runs are a small sample.

- The CI job cases (make test-live-agents-md) failed in 3 of 6 runs on this
  branch: its clean case got fail: non_inferable. Grader noise, not this
  branch: the clean case's 3 non_inferable calls, 24 runs per tree, failed
  4/24 on the branch and 6/24 on origin/main 8ad2720, each time 2 of 3 haiku
  calls calling a different CLAUDE.md line (a Map row, the lint rule, the
  go-os-tests bullet) a restatement of a doc comment. At about 20% per run the
  job fails often on any PR.

- The Claude ask hook now holds one GET /need on the service's unix socket
  <main>/.pm/run/wait.sock instead of dialing the store each second (ec2feb0):
  idle, 1 call and 0 store dials (fake); woken 24 ms after a site reply's POST
  with the service's periodic look set to 1 h; a close elsewhere wakes it
  within 1 s (ServeCheck). Live ask scenarios 4/4; Claude site-answer latency
  0.5 s (was 1.0 s).

- Claude Code 2.1.296, pty, 2 runs per case: when an AskUserQuestion dialog
  ends, PostToolUse fires 17-24 ms after a chosen answer (tool_response holds
  questions and answers; stdin has session_id, prompt_id, tool_use_id,
  tool_input) and after a hook's answer; on Esc no hook fires at all (not
  Stop; PermissionDenied fires only on an auto-mode classifier denial), but
  the running PermissionRequest hook gets SIGTERM 79-81 ms after the dialog
  closes and is killed about 1.2 s later; a session stopped with the dialog
  open sends it SIGTERM at +21 ms; a chosen answer sends no signal; no hook
  process outlives the session.

- Questions close from hook events, no transcript or rollout read (72b703a):
  Claude PostToolUse closes a dialog answer, SIGTERM (Esc, session stop)
  dismisses, Codex UserPromptSubmit closes a typed answer, the service closes
  before codex queue. Parity -k ask_tool, 5 passed (Esc scenario added): each
  chat answer and the Esc closed the need by the first check after the key
  press (<0.5 s; Codex was 41.5 s via the sweep); site answer reached Claude
  in 0.5 s, Codex in 5.1 s. Whether a codex-queued message fires
  UserPromptSubmit was not observed; the service's close before the queue and
  Relayed make it harmless either way.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: an ask-tool question (Claude Code's AskUserQuestion, Codex's request_user_input_async) is a question need shown on the site under "Sessions ask you", answerable there or in the session's chat, and the first answer reaches the session.

- A fourth need kind, `question`, with its questions and options (schema 4); only an open question may lack a parent.
- Raising: `pm hook ask --runtime claude` on PermissionRequest of AskUserQuestion, beside the open dialog, 24 h timeout; `pm hook ask --runtime codex` on PostToolUse of request_user_input_async, keyed by its call id.
- Closing from hook events, never from a transcript or rollout read: Claude Code's PostToolUse of AskUserQuestion closes a dialog answer; the SIGTERM Claude Code sends the waiting hook on Esc or a stopped session closes the question as dismissed; Codex's UserPromptSubmit closes the session's open questions with the typed prompt.
- The Claude hook waits for a site answer on one blocking call, `GET /need` on the service's `<main>/.pm/run/wait.sock`, woken by the service's snapshot instead of a store read each second; for Codex the service closes the question, then runs `codex queue`, so the close decides which answer wins.
- The site's question card: options as radios or checkboxes, an Other box, one `<question>: <answer>` line per question.
- pm's request rule picks the form by what is asked, not by whether the owner is in the chat: a question through the ask tool, a decision as a decision need, an action as an action need. AGENTS.md: read a session's transcript only when no hook event or input gives the fact.
- The design page [Owner questions](../design/owner-questions.md), and [Reply delivery](../design/reply-delivery.md) restructured by request kind and runtime.
- Left out: a dismiss control on the site's question card (a question from an abandoned Codex session stays until answered or `pm need dismiss`); Codex's blocking request_user_input (Plan mode).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Live check in a scratch clone, each runtime in a TUI under a pty: met. `PM_PARITY_CODEX_SANDBOX=danger-full-access PM_LIVE_TESTS=1 uv run pytest -n 4 tests/test_agent_parity_live.py -k ask_tool` after the hook-event closes (72b703a, f02028d): 5 passed. Answered in chat, on each runtime: the question closed with the chat answer by the first check after the key press (under 0.5 s; it was 41.5 s for Codex through the sweep before), nothing delivered, and a later POST from the stale card refused as already closed. Esc on Claude Code's dialog: the question closed as dismissed in under 0.5 s. Answered on the site: the answer reached Claude Code in 0.5 s (1.0 s before the wait socket) and Codex in 5.1 s.
- The request rule says which tool fits which kind, in its three texts, with judge cases: met. 9467a0f; 3 cases added, none moved; `make test-live`, 78 cases x 3 runs: Claude 234/234 and Codex 234/234 in two clean runs (earlier runs and the prompt fix they led to are in Findings).
- `make test` and the PR's CI green, with harness tests for both races and both runtimes: `make test` on f02028d: 143 passed, 169 skipped (live-model and integration tests). CI on PR #89: all 13 checks pass at 857f81e, and `make merge-ready PR=89` passes.
