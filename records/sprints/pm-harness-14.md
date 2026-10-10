---
type: sprint
title: Hooks for the pm harness
bead: yeeef-agents-9va.14
---

## Goal

> What should be true when this sprint ends, and why now?

Know which agent hooks the pm harness should use, and why: for example starting a session with `pm show`, reminding an agent to record findings or close tasks before it stops, or checking records before a commit. Today the only hook is Beads' `bd prime --hook-json` at SessionStart, and the harness relies on agents following RULES.md unprompted.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the hook events Claude Code and Codex offer and what each could do for the harness (context at session start, checks at stop or commit, reactions to tool use); prior art, including how Beads uses hooks; a recommendation with costs, written up as a doc; candidate tasks for building it.

**Out:** building the hooks (a later sprint, after the owner picks); hooks for context size, such as output caps, which belong to the context-efficiency project.

## Done when

> What evidence will show the goal is met?

- A doc lists each candidate hook with its event, what it does for the harness, its cost (latency, tokens, failure modes) and whether Claude Code and Codex both support it; claims are checked against the docs or a real run.
- The owner has a recommendation to accept or change, raised as a need.

## Design pages

> Where is the detail?

- [Owner-request Stop hook](../design/owner-request-hook.md): how the hook judges a final reply, and the alternatives (patterns, Haiku, Jev)

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-04}
The owner-request Stop hook judges with a fast model, not phrase patterns: in Claude Code it is a prompt-based hook (`"type": "prompt"`, Haiku) that blocks a final reply asking the owner for a decision or action without citing a need or action id; Codex, which has no prompt hooks, keeps the pattern script.
Patterns missed requests phrased differently and blocked replies that cited their needs; a model reads intent, and the prompt hook runs on the session's own auth with no script to maintain.
:::

::: decision {source=owner date=2026-10-04}
Try Claude Haiku first: in Claude Code the owner-request Stop hook becomes a prompt-based hook judged by Haiku; Jev stays a documented alternative, and Codex keeps the pattern script for now.
Haiku needs no new key or third party and covers the main runtime; the design page compares the alternatives so the choice can be revisited with measurements.
Answers `yeeef-agents-9va.14.4`.
:::

::: decision {source=agent date=2026-10-05}
The uncommitted-records Stop hook blocks only on store files this session's tool calls name by their path under the store (Claude Code tool_use inputs; Codex function_call arguments and custom_tool_call inputs), not on every uncommitted file; with no readable transcript it lets the stop through.
Other sessions write the same store at once and a session commits only its own records, so blocking on any dirty file would push an agent to commit or revert another session's edit in progress; a hook cannot know for certain which session wrote a file, and a missed own edit (one made after cd into a store folder, through a glob, or by a subagent) costs only an uncommitted record that the next pm write to it refuses, which is cheaper than a wrong commit.
:::

::: decision {source=owner date=2026-10-05}
The owner-request Stop hook blocks only a request that sprint work waits on (a decision on a sprint's scope or design, a PR review or merge, an action a task is blocked by) asked without a need or action id; conversational questions, offers and clarifications pass.
The owner found the hook too rigid in live conversation; requests tied to sprint work are the ones that must reach the site.
Answers `yeeef-agents-9va.14.7`.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Measured (3 runs each, this repo, 2026-10-04): `bd prime --hook-json` takes
  0.25 s warm (0.54 s cold) and emits 6,923 bytes (~1,730 tokens at bytes/4);
  `bin/pm show` takes 0.81 s and emits 2,433 bytes (~610 tokens); `pm show
  --json` 0.82 s, 5,801 bytes (~1,450 tokens). A SessionStart `pm show` adds
  under 1 s and ~600 tokens per session.

- Nothing checks generated sections today: site.py inserts generated Progress
  after the section's prompt lines and leaves any hand-written text below it,
  and render does not fail on it. A render check would catch writes from any
  tool and any agent, which a PreToolUse hook on Edit/Write cannot (Bash and
  Codex edits bypass it).

- Codex 0.131.0 has hooks on by default with 8 events (SessionStart,
  UserPromptSubmit, PreToolUse, PermissionRequest, PostToolUse, PreCompact,
  PostCompact, Stop; no SubagentStop), in Claude Code's JSON shape, command
  handlers only, each gated by a one-time trust approval in /hooks. No Codex
  hook is configured here, so Codex sessions get no `bd prime` today.

- PreCompact cannot inject context in Claude Code; SessionStart re-fires with
  matcher `compact` after compaction, so a SessionStart hook already restores
  pm state. Claude Code caps a hook's additionalContext at 10,000 characters
  (bd prime is 6,923 B, pm show 2,433 B).

- Haiku prompt Stop hook (claude-haiku-4-5-20251001, Claude Code 2.1.289) adds
  0.9-2.1 s per stop, 1.4 s mean over 4 stops, measured in debug logs from
  hook start to model response (2.11 s, 0.88 s, 1.53 s, 1.07 s).

- Live check of the Haiku Stop hook in three claude --bg sessions: (a) a reply
  ending 'needs input: should we rename X? please decide' was blocked (ok
  false), the session raised decision need yeeef-agents-9va.14.5 and cited it,
  then passed (dismissed after); (b) a reply citing open need
  yeeef-agents-9va.15.3 passed; (c) a plain status reply passed.

- Claude Code wraps a Stop prompt hook as a stopping condition over the
  transcript and substitutes the whole Stop input (stop_hook_active,
  last_assistant_message) for $ARGUMENTS; it still calls the model on
  stop_hook_active, and the blocked agent sees the full prompt followed by the
  reason as Stop hook feedback.

- Built hooks measured (2026-10-05, Claude Code 2.1.289): the SessionStart pm
  show hook took 1,407 ms and 1,141 ms in two live sessions, in parallel with
  bd prime (1,897 ms and 348 ms), so it adds 0-0.8 s per start, and injects
  4,359 characters (~1,100 tokens at bytes/4; pm show has grown from 2,433 B
  at the survey); the uncommitted-records Stop hook takes 0.05 s on a clean
  store (3 runs), 0.08 s blocking with a 21 MB transcript, and 69 ms on the
  stop_hook_active pass in a live session.

- Live checks in claude --bg sessions from the pm-hooks worktree: (a) a
  session quoted the injected 'Project state from bin/pm show' context
  verbatim; (b) a session that hand-edited a [TEST] finding into this record
  and stopped was blocked once (reason naming
  records/sprints/pm-harness-14.md), committed it with bin/pm commit
  (275f6c4), and its next stop passed; the finding was removed in 66b5cfb.

- Narrowed Haiku owner-request hook, live in four claude --bg sessions (task
  9va.14.8): a sprint request without an id ('PR #99 ... Please review and
  merge it.') was blocked and the session raised and cited [TEST] action
  yeeef-agents-9va.14.9 (dismissed after); an offer ('If you want a Codex live
  check too, tell me'), a clarifying question about the user's message, and a
  sprint request citing yeeef-agents-883.1.5 passed; the judgement took
  0.7-2.5 s per stop (701, 2190, 2502, 1427 ms).

- Codex live check after trust (codex-cli 0.160.0, codex exec from the main
  checkout, 2026-10-05): SessionStart and UserPromptSubmit hooks ran, but no
  Stop hook ran at all, so a 'please review and merge' reply passed.
  ~/.codex/config.toml trusts only stop:0:0 and session_start:0:0 of this
  repo; the new session_start:0:1 and stop:0:1 entries are untrusted, and even
  the trusted stop:0:0 did not fire, which suggests codex exec does not run
  Stop hooks. An interactive Codex session is the next thing to check.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: the pm harness uses hooks where they pay off: session start shows
`pm show`, a turn cannot end with this session's records uncommitted or with
an unraised request that sprint work waits on, and `make render` rejects
hand-written text in generated sections.

- Hooks survey with measured costs:
  [Hooks for the pm harness](../docs/2026-10-04-hooks-for-pm.md).
- Session start injects `pm show` beside `bd prime` (Claude Code and Codex):
  about 1,100 tokens, 0–0.8 s added per start.
- Stop: blocks once on uncommitted records this session's tool calls named
  (0.05–0.08 s); another session's files never block.
- Stop: the owner-request hook is judged by Haiku in Claude Code and blocks
  only requests sprint work waits on (0.7–2.5 s); Codex keeps the pattern
  script ([design](../design/owner-request-hook.md)).
- `make render` fails on text in Progress or on a generated heading written
  by hand.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- A doc lists each candidate hook with its event, benefit, cost and
  runtime support, checked against docs or a real run: met.
  [Hooks for the pm harness](../docs/2026-10-04-hooks-for-pm.md) covers
  seven candidates; Claude Code claims cite the hooks docs, Codex claims the
  rust-v0.131.0 source; `bd prime` and `pm show` timings and token counts
  were measured here (+0.81 s, about 610 tokens).
- The owner has a recommendation to accept or change, raised as a need: met,
  `yeeef-agents-9va.14.2` with four options and a default.
- Owner-request Stop hook judged by a fast model (task `9va.14.3`, added
  after the survey): met. Three live `claude --bg` sessions: a chat-only
  request was blocked and the session raised and cited a need; a reply
  citing an open need passed; a status reply passed (31623ff).
- The recommended hooks are built (task `9va.14.6`, after the owner accepted
  the recommendation on `9va.14.2`): met. Live `claude --bg` sessions: one
  quoted the injected `pm show` context; one left an uncommitted record, was
  blocked once, committed it and stopped; `test_hooks.py` covers each hook
  (1e1b2b0, c273d4f, 8b567c2).
- The owner-request hook blocks only sprint-work requests (task `9va.14.8`,
  owner decision on `9va.14.7`): met. Live: a PR review request without an id
  was blocked and raised; an offer, a clarifying question and a request
  citing an id passed (3178d7c).
