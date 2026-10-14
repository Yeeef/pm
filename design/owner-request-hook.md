---
type: design
title: Owner-request Stop hook
project: pm-harness
covers:
  - "internal/hooks/ownerrequest.go"
  - "internal/hooks/store.go"
  - "internal/cli/ownerrequest.go"
  - "internal/ask/**"
  - "internal/hooks/ask*.go"
  - "prompts/owner_request_*.txt"
---

## Problem

> What are we solving, and why now?

- Agents ask the owner for decisions and actions in chat ("needs input: …", "please review …"). When the owner is not watching the chat, those requests never reach the site, so the owner may never see them.
- In context-efficiency sprint 1, four of six things waiting on the owner were asked only in chat, although the rules already required raising them. A rule the agent must remember did not work; the owner wants a check that runs on its own when an agent ends its turn.
- The rule and the check must say the same thing. On 2026-10-10 the hook blocked three of four replies in a conversation the owner was having with an agent: it judges every request, while prime.md and the block text claim a "sprint work waits on it" condition that nothing checks (pm-harness sprint 107, Findings).

## Goals and non-goals

> What must the design achieve, and what does it deliberately leave out?

Goals:

- An agent cannot end its turn with a request to the owner left only in chat. Each request is raised as a need first: a decision, an action or a PR review, under the sprint or task it belongs to; with none, the agent opens one first.
- When the agent has a clear reason not to raise a need, such as the owner plainly being in the chat and wanting to answer there, it asks with the runtime's ask tool (AskUserQuestion in Claude Code; `request_user_input_async` in Codex, `request_user_input` in its Plan mode), never as a question in plain text.
- An ask-tool question is not lost when the owner steps away from the chat: pm raises it as a question need, which shows on the site, and the first answer, in the chat or on the site, reaches the session.
- The agent does what it can itself first, and asks the owner only for what only the owner can do.
- One rule in three texts: prime.md's request rules, the judge prompt and the reprompt allow and block the same cases.
- Few false alarms: a reply that quotes, reports or describes a request without asking it, and an offer nothing waits on, pass.
- Cheap enough to run at every turn end: a few seconds at most.

Non-goals:

- A per-session mode that turns the hook off while the owner is in the chat: the runtime's ask tool covers that case (Alternatives considered).
- Checking subagent reports: they end with SubagentStop and go to the main agent, not the owner.
- Proving a matched need is the right one: a semantic match to an open need of this session is enough.

## Constraints and key facts

> Which facts, findings and constraints shaped the design? Only those that
> still hold; sprint records keep the findings as they happened.

- A Stop hook sees the final reply (`last_assistant_message`), `session_id` and `cwd`, and can block once; on `stop_hook_active` the next stop must pass, so the hook can never loop.
- AskUserQuestion runs mid-turn and waits for the answer, so its question is never in `last_assistant_message`. Codex's `request_user_input_async` (Default mode, gpt-6 models) returns `{"accepted":true}` at once and the answer arrives later as a new user message, so the turn ends with the question open, and its text can be in the final message (in `codex exec` it was). `request_user_input` (Plan mode) blocks, with no timeout. In a session nobody watches, a blocking ask waits forever. AskUserQuestion's and `request_user_input_async`'s questions show on the site as question needs (Ask-tool questions on the site); `request_user_input`'s do not.
- The transcript records each ask: Claude Code's AskUserQuestion `tool_use` (input `questions[].question`), Codex's `function_call` named `request_user_input_async` (arguments `{"questions":[{"title":…,"options":[…]}]}`, measured live) or `request_user_input` (`questions[].question`).
- The Claude judge, `claude -p --model claude-haiku-5-5` with thinking off and no settings, tools or MCP, takes about 2 s; with thinking on a verdict took 5 to 36 s. Its timeout is 15 s, inside the hook's 30 s.
- Each judge runs in a process group of its own, killed whole at its timeout (both CLIs are launcher scripts), and killed by the hook when the runtime stops the hook (SIGTERM, SIGINT, SIGHUP) at its entry's timeout.
- The Codex judge, `codex exec --model gpt-5.6-luna` at medium reasoning effort, takes about 5.5 s (p90 14 s, 8 calls at a time); at low effort it missed 3 or 4 of 189 labelled runs. Its timeout is 40 s, inside the Codex entry's 45 s.
- A `codex exec` child runs the hooks it finds, its own Stop included, so a judge that ran them would run the judge again. `--disable hooks` turns Codex's hook engine off; `--ignore-user-config` drops `$CODEX_HOME/config.toml` (its hooks, MCP servers, notify); `--ignore-rules` drops execpolicy rules. `$CODEX_HOME/AGENTS.md`, the user's own instructions, still loads: no flag turns it off (codex-rs 0.154 `codex-home/src/instructions`).
- The judge sees only the reply and this session's open needs (title and the first 400 characters of the description). It knows nothing of sprints or tasks, so a rule that depends on them cannot be enforced.
- A need's `--parent` must be an open item inside a project with a record. A project itself is refused ("not inside a project with a record"), so every need sits under a sprint or a task.
- Codex runs only command hooks, and trusts each hook per file path and hash: a new or changed hook entry runs only after the owner approves it in `/hooks`.
- Codex's Stop payload differs from Claude Code's in one key the hook reads: `last_assistant_message` is nullable (null for a turn that ended without a message). `session_id` is the root session's in both runtimes.
- A Stop hook that fails (exit 1) lets the stop through in both runtimes; Codex does so silently. A check that cannot run is therefore lost, not blocking: so each session's judge runs on the runtime that session already has.
- Phrase patterns (sprint 13) missed requests worded differently and blocked a reply that cited every request because one line said "waiting on you".
- Agents cite requests by the short id `pm show` prints (`9va.29.4`). An id written in a reply is not a match by itself: only an open need of this session matches.

## Design

> What is it, in its final state? Free `###` subsections. Decisions and plans
> live in the project and sprint records.

### Overview

```mermaid
flowchart TD
  S[Stop: pm hook owner-request] --> A{stop_hook_active, or a blank,<br/>null or missing reply?}
  A -- yes --> P[pass]
  A -- no --> N[read this session's open needs]
  N --> J[the judge on the session's runtime<br/>labels each sentence and matches it to a need]
  J --> B{unmatched decision, review,<br/>action or clarification;<br/>or a needless ask?}
  B -- no --> P
  B -- yes --> K[block with the reprompt]
```

Reading: every reply is judged; a question asked with the ask tool reaches the judge as an open request the owner sees (this turn's from the transcript, any turn's as an open question need), so neither it nor its repeat in the reply blocks.

### One rule

The rule, as all three texts state it:

1. Do what you can yourself first. Ask the owner only for what only the owner can do: a choice, a credential, a review or merge, a step outside the agent's reach.
2. Raise each such request as a need under the task or sprint it belongs to. If no sprint or task holds it, open one first (`pm task add`, or `pm sprint open` for work no open sprint fits).
3. When there is a clear reason not to raise a need, such as the owner plainly being in the chat and wanting to answer there, ask with the runtime's ask tool (AskUserQuestion in Claude Code; `request_user_input_async` in Codex, `request_user_input` in its Plan mode), never as a question in plain text. pm raises the question on the site too; its answer steers the session and records no decision.
4. Do not ask leave for a step you are authorized to take: opening the project, sprint or task for work the owner asked for, working a sprint's tasks, committing, pushing its branch, opening its PR. The owner's request is the leave to open; a request that states the goal confirms it. Asking what an unclear goal is stays a clarification, and asking whether to take on work the owner did not ask for stays a decision.

| Text | Where | States the rule as |
|---|---|---|
| prime.md | the "Never leave a request only in chat" invariant | the rule in one line: ask only for what you cannot do yourself, as a need under its task or sprint (opened if none holds it), or with the runtime's ask tool, named per runtime in one clause; item 4 is the two invariants that agents push and open PRs, and open the project, sprint or task for work the owner asked for, without asking. The detail is left to the reprompt, which the agent gets whenever it matters, to keep every session's context small |
| Judge prompt | `owner_request_system.txt` | what to block: an unmatched decision, review, action or clarification, and a needless ask (items 2 and 4); what passes: offers nothing waits on, and quotes, reports and plans |
| Reprompt | `owner_request_reason.txt`, `owner_request_needless.txt` | how to get past a block: items 1, 2 and 3 in that order; item 4 for a needless ask |

- prime.md is short and the reprompt is detailed, but no text adds a condition the others lack. The "sprint work waits on it" condition goes from all three: the judge cannot check it, and a chat request is lost whether a sprint holds it or not.
- A rule change edits all three in one commit, and adds a labelled case to `tests/owner_request_cases.json` for each case it moves.

### The judge

- `pm hook owner-request --runtime claude|codex` reads the session's open needs from the work store, then asks a small model on that runtime for every sentence of the reply that addresses the owner, its kind, and the open need it matches, by meaning.
- Kinds that block when no open need matches: `decision`, `review`, `action`, `clarification`. A sentence that makes the agent's next step depend on something the owner must first provide or do is an `action`, worded as a condition ("If you paste the log, I'll find the cause") or a statement ("Without the log I can't tell which"). A clarification asked in plain text is lost when nobody reads the chat; asked with AskUserQuestion it is not in the reply.
- Kinds that pass: `offer` and `suggestion` that nothing waits on, and `not asked` (quotes, reports, examples, the agent's own plan, text in code blocks). An offer the work cannot finish without is a request, however politely put.
- `authorized` (a needless ask) blocks even when a need matches.
- The questions the session asked with its ask tool in the stopping turn (read from `transcript_path`: after the user's last prompt in Claude Code, after the last `turn_context` in Codex) join the open requests as entries marked "(asked with the ask tool this turn)", so a sentence asking the same thing matches one. The judge still runs for the turn: another ask in the same reply, left only in chat, blocks. An earlier turn's question still open is an open question need of the session, so a reply that repeats it matches it.

### Ask-tool questions on the site

`pm hook ask --runtime claude|codex` raises each ask-tool question as a question need ([Work store](../design/work-store.md), Data model), under the newest open task the session holds, else under none. The owner answers it in the chat or on the site's "Sessions ask you" card ([Replies on the site](../design/site-replies.md)); the first answer closes the need, and the other is dropped ([Reply delivery](../design/reply-delivery.md), Questions).

| | Claude Code | Codex |
|---|---|---|
| Entry | PermissionRequest, matcher `AskUserQuestion`, `pm hook ask --runtime claude \|\| exit 1`, timeout 86,400 s | Stop, first: `pm hook ask --runtime codex \|\| exit 1`, 30 s |
| When it raises | at the call: Claude Code runs the hook beside the question's dialog, without waiting for it | at turn end: `request_user_input_async` returns at once, so the hook raises each such call of the turn once, by its `call_id` |
| Key | `prompt_id` with a hash of the tool input (the hook input has no tool-use id) | `call_id` |
| Chat answer | the transcript's tool result: the answer closes the need; a rejected dialog dismisses it | the first user message after the call: `pm hook inbox` and the service's sweep close the need with it |
| Site answer | the hook, which polls every second, closes the need and prints `allow` with `updatedInput.answers`, which closes the dialog | the service queues it with `codex queue` |
| A subagent's call | steps aside | not raised (a subagent's Stop is SubagentStop) |

- A question's answer is an instruction to the session, not a ruling: `pm decision add --need`, `pm decision close`, `pm action done` and `pm need edit` refuse a question, naming `pm need dismiss`. The agent records a decision with `--confirmed` only when the answer rules the work.
- The hook fails open: on its own error it says why on stderr, prints nothing and exits 0, so the dialog stays usable and the stop goes on.
- `request_user_input` (Codex's Plan mode) blocks inside Codex's app-server, which a second client reaches only with `app-server --listen` and a TUI started with `--remote`; it is not raised.

### The reprompt

The block reason the agent sees (`owner_request_reason.txt`), in this order:

1. The quoted sentences that blocked.
2. Do it yourself first: read the file, run the command, find the transcript, take the step the rules authorize. Only what only the owner can do is a request.
3. For each request left, raise it, with the `pm decision need`, `pm action need` and `pm action need --pr` forms; the parent is the task or sprint it belongs to. If there is no such sprint or task, create one first (`pm task add`, or `pm sprint open` for work no open sprint fits).
4. If there is a clear reason not to raise a need (for example, the owner is plainly in the chat and wants to answer there), ask with the session's ask tool, never as a question in plain text: the reason names AskUserQuestion for a Claude Code session and `request_user_input_async` (or `request_user_input` in Plan mode) for a Codex one, from the hook's `--runtime`.
5. Then end the reply, with no plain-text question to the owner left in it.

The needless-ask reason (`owner_request_needless.txt`): take the authorized step instead of asking; it lists the same authorized steps as item 4 of the rule, committing included.

### Runtimes

| | Claude Code | Codex |
|---|---|---|
| Entry | `.claude/settings.json`, Stop | `.codex/hooks.json`, Stop (runs once the owner trusts pm's hooks; a session whose start hooks ran is told at its start when Codex skips this one, as after a `pm upgrade` changed it) |
| Command | `pm hook owner-request --runtime claude \|\| exit 1`, 30 s, before `pm hook stop` | `--runtime codex`, 45 s, after `pm hook ask --runtime codex` (30 s) and before `pm hook stop` |
| Reply | `last_assistant_message`, a string | the same key, nullable: null passes |
| Judge | `claude -p`, Haiku, 15 s | `codex exec`, `gpt-5.6-luna`, 40 s, from an empty temp dir with hooks, rules and the user's config off and no saved session (`--ephemeral`); needs no `claude` |
| A check that cannot run | exit 1 with the cause on stderr; the stop goes through | exit 1; Codex passes the stop silently |
| Ask tool (rule item 3) | AskUserQuestion | `request_user_input_async` (Default mode), `request_user_input` (Plan mode) |

- Each entry names its runtime; pm never guesses it. An entry without `--runtime` runs the Claude judge, as every entry did before the flag.
- One judge prompt serves both runtimes, and every labelled case must pass on each.
- `pm doctor` does not check either judge's CLI: a session's judge runs on its own runtime, which runs only where it is installed.
- A Codex subagent's SubagentStop runs only `pm hook stop`; the owner-request check never judges a subagent's report (Non-goals).
- The Stop entry runs only the two checks, after, in Codex, `pm hook ask`, which raises the turn's ask-tool questions first so the judge reads them as open needs. `pm hook inbox`, which hands over the owner's replies, runs at UserPromptSubmit, never at Stop: an inbox block at Stop forces a continuation that ends with `stop_hook_active`, so this hook would pass the very turn that acts on the reply ([Reply delivery](../design/reply-delivery.md)).

### How it is checked

- Review: prime.md, the judge prompt and the reprompt read side by side; no case is allowed by one and blocked by another.
- `make test-live` on `tests/owner_request_cases.json`, on each runtime's judge, each case 3 runs by default (`PM_LIVE_RUNS`), all with the case's verdict (2026-10-13, 68 cases: Claude 204/204, median 2.35 s, p90 4.08 s; Codex 204/204, median 5.77 s, p90 14.74 s, max 33.79 s); after a prompt change, run 10, since a case at 4 in 10 can pass 3 of 3 by chance. Cases include the three replies blocked on 2026-10-10, labelled by the rule (a chat request with no sprint blocks), and a reply whose only question went through AskUserQuestion (passes).
- No recursion: the harness's fake `codex` logs the judge's argv and directory; live, a user Stop hook in `$CODEX_HOME/hooks.json` ran under a plain `codex exec` and did not under the judge's.
- The parity live eval: a Codex session with no `claude` on its PATH that asks the owner only in chat is blocked, then raises a need.
- A live session: a reply that asks the owner for a file only in chat is blocked and the reprompt names doing it yourself first; it passes once a need is raised under a sprint or task, or once the question moves to the ask tool.
- The parity live eval's ask tool scenario: an interactive session (Claude Code's and Codex's TUI) asks a multiple-choice question with its ask tool, the test picks an option at the keyboard, and the Stop hook does not block; the question need pm raised closes with the keyboard's answer.

## Alternatives considered

> What else was considered and not adopted, and why not?

| Option | How it works | Why not chosen |
|---|---|---|
| **Keep the sprint-work condition** | Exempt requests no sprint waits on | The judge cannot see sprints or tasks, so the condition was never checked; and a chat request in an unattended session is lost whether a sprint holds it or not |
| **Per-session interactive mode** | The owner sends `pm interactive`; the hook reads the last such prompt from the transcript and skips the judge | AskUserQuestion already keeps a live question out of the judged reply; the mode adds a transcript reader for two runtimes and open questions on Codex's transcript format and on compaction, for one block-and-retry per plain-text question (owner decision 2026-10-10) |
| **Pass when the session holds no claimed task and no open need** | Deterministic gate before the judge | Lets every request through in a planning or Q&A session where the owner may not be watching |
| **Needs filed under the project** | A request outside any sprint becomes a project-level need | The owner ruled that every request is tracked work: open a sprint or task first (2026-10-10); pm also refuses a project as `--parent` |
| **Claude Code prompt hook** (`"type": "prompt"`, the earlier design) | Haiku reads the reply in-process | Cannot read the work store, so it checked that an id was cited, not that the need was open; Claude Code only |
| **Phrase patterns** (sprint 13) | A phrase list plus a cited-id check | Missed requests worded differently; blocked a reply that cited its needs |
| **Rule only** | The agent remembers | Four of six requests were asked only in chat |
| **`claude -p` for every runtime** (sprint 1) | Codex sessions call the Claude judge | A Codex-only machine has no `claude`; the judge fails at every stop and Codex passes each silently (owner decision 2026-10-12) |
| **Skip the judge for a turn with an ask tool call** | Any ask tool call in the turn lets the stop through | Another ask left only in chat in the same reply would pass; giving the tool's questions to the judge as asked keeps it judged |
| **An ask-tool question as a decision need** | The hook raises a decision need for the question | A decision rules a project's or sprint's outcome and is recorded; a question steers one session and its answer is not recorded, so a decision need would demand a decision record for every answer (owner decision 2026-10-14) |
| **Fall back to the other runtime's CLI** | Try codex when claude is missing, or the reverse | Hides a missing runtime and changes which model judges; the entry names the runtime instead |

## Prior art

> Optional. What existing work did we learn from, and what did we take?

- The hooks survey for this project ([Hooks for the pm harness](../docs/2026-10-04-hooks-for-pm.md)) measured hook costs and found that Codex has no prompt hooks.

## Open questions

> What is still unresolved?

1. **An unattended ask tool:** the agent's judgement of a "clear reason" is the only guard against a question that waits forever in a session nobody watches. If agents misjudge it, the rule narrows item 3.
2. **Sync and async requests:** a request's kind (question, decision, action) says what the owner gives; its timing says who waits. A sync request is one a session waits on now: light in form, tied to that session, closed by the first answer in chat or on the site. An async request is one the session moves on from, answered later by whichever session takes the reply. The axes are independent: an action can be sync ("log in to `gh`, then I continue"), and a decision can be asked while the owner is in the chat. Not built: kept as a lens for when timing, not kind, has to drive what the site shows or how a reply is delivered (sprint pm-harness-118).
