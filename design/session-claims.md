---
type: design
title: Session claims
project: pm-harness
covers:
  - "internal/cli/agent.go"
  - "internal/cli/store_commands*.go"
  - "internal/cli/writes.go"
  - "internal/cli/task.go"
  - "internal/cli/move.go"
  - "internal/cli/clean.go"
  - "internal/cli/show.go"
  - "internal/work/dolt.go"
  - "internal/work/ready*.go"
  - "internal/hooks/prime.go"
  - "internal/session/**"
  - "internal/cli/session_test.go"
---

## Problem

> What are we solving, and why now?

Part of the [Work layer: Beads](work-layer.md) design. Several agent sessions, in Claude Code and in Codex, work one clone at once. An assignee name is the same for every session of one user, so two sessions could take the same task and neither `pm show` nor the owner could tell who holds what or whether that holder is still running. Each command must also know which session runs it, though a Codex run can inherit a Claude Code session's environment and a Codex subagent is a thread of its own.

## Goals and non-goals

> What must the design achieve, and what does it deliberately leave out?

**Goals:**

- Each command resolves the one session that runs it, in either runtime, or refuses.
- A subagent acts as its session: in both runtimes it holds what its session holds.
- Each claim names the session holding it and when it was taken.
- A claim held by a live session cannot be taken by another session, in either runtime; a claim left by a finished session can.
- `pm show` shows who holds each task and whether that session is live.

**Non-goals:**

- Locking beyond the claim: the claim is a compare-and-set in one work-store write.
- Coordinating sessions on different machines (see Open questions).

## Constraints and key facts

> Which facts, findings and constraints shaped the design? Only those that
> still hold; sprint records keep the findings as they happened.

| Fact | Claude Code | Codex (0.154) |
|---|---|---|
| Session id in commands | `CLAUDE_CODE_SESSION_ID`, shared with subagents | `CODEX_SESSION_ID`, the root session's, shared with subagents; `CODEX_THREAD_ID` is the running thread's (an earlier Codex sets only this) |
| Session id in hooks | payload `session_id` | payload `session_id`, the root session's; no `CODEX_*` variables in the hook's environment |
| Transcript | `<config dir>/projects/<project dir>/<id>.jsonl` (`$CLAUDE_CONFIG_DIR`, default `~/.claude`) | rollout `$CODEX_HOME/sessions/YYYY/MM/DD/rollout-<time>-<thread>.jsonl`, in the directory of the thread's start date in local time |
| A subagent's link to its root | same id | the rollout's first line, `session_meta`: its `session_id` (the root) and `parent_thread_id` |
| Process | `claude` (or `…/claude/versions/<v>`), its pid exported as `$CLAUDE_PID` | `codex`, or a `codex-*` helper |

- Codex keeps its launcher's environment: a Codex run started from Claude Code sees `CLAUDE_CODE_SESSION_ID`, and a Claude Code run started from Codex sees Codex's ids.
- A Codex thread id is a UUIDv7, whose first 48 bits are its start in Unix milliseconds.
- A transcript is rewritten while its session runs; its modification time is the only local liveness signal. Transcripts are local files, so liveness can only be read on the machine that runs the session.

## Design

> What is it, in its final state? Free `###` subsections. Decisions and plans
> live in the project and sprint records.

### Session resolution

A command's session, from its environment:

| Set | Session | Runtime |
|---|---|---|
| Only `CLAUDE_CODE_SESSION_ID` | it | Claude Code |
| Only Codex's | `CODEX_SESSION_ID`; else `CODEX_THREAD_ID` mapped to its root | Codex |
| Both runtimes' | the innermost runtime's: the nearest ancestor process that is Claude Code or Codex | that runtime |
| Both, and neither runtime among the ancestors | none: the command refuses, naming both ids and which variables to unset | |
| Neither | no session; a command that needs one (`pm task claim`) fails hard | |

- **Root of a thread.** `CODEX_THREAD_ID` maps to its root through the `session_meta` of its rollout, then of its parent's, and so on, so a subagent shares its session's id, as in Claude Code.
- **Ancestry.** Read once per command, from `/proc` on Linux and `ps` on macOS, at most 64 levels up.
- **Ambiguous refuses everywhere.** Every command that may act as a session refuses an ambiguous one before it runs, the self-parsing store commands included; `version`, `prime`, `hook` and `--help` act as none. A hook reads its session from its input payload, never from the environment.
- **Session-start context.** The SessionStart hook runs `pm show --refresh-inbox` as the starting session: in Claude Code with the hook's own environment; otherwise (Codex) with `CODEX_SESSION_ID` set to the payload's `session_id` and Claude Code's session variables removed.
- **Codex hooks skipped.** A Codex session's first command (not `hook`, `prime`, `version` or `--help`) checks once whether Codex ran pm's session-start hooks in it: whether its root rollout holds a developer message opening with pm's rules title (`# pm rules (`) before the first user message after the first `turn_context` (the user messages before it are Codex's own context). Ran none: one warning on stderr naming the fix (trust pm's hooks in `/hooks`). It reads at most the rollout's first 1 MiB, says nothing while it cannot tell (no rollout, first prompt not yet written), never fails the command, and keeps each session's answer in `<main>/.pm/run/codex-hooks/<id>`.
- **The inbox follows the runtime.** Only a Claude Code session stores a messaging socket on the needs it raises ([Reply delivery](../design/reply-delivery.md)).

### Claiming

`pm task claim <id>` makes the session (`--session`, else the resolved one) the task's holder, with the time, in one compare-and-set write ([Work store](../design/work-store.md)). It fails hard when no session id is available, and refuses in the main checkout. The same session, its subagents included, may re-claim. There is no `--force`.

### Liveness

A holder is live when one of its transcripts changed within `LiveWindow` (30 minutes): a Claude Code transcript named `<id>.jsonl` under any project dir (each worktree has its own), or a Codex rollout of the thread `<id>`. A rollout is looked up only in the date directories its UUIDv7 start can fall in across time zones (UTC-12 to UTC+14), not by walking every rollout; an id that is no UUIDv7 is looked up in every date directory.

A claim held by a live other session is refused, naming the holder, its age and the live window; one held by an idle session is taken over, and the output says so. This holds across runtimes: a Codex session cannot take a live Claude Code session's task, and the other way round.

### Live sessions in worktrees

`pm clean` keeps an agent worktree that a live session used: the latest entry, written within `LiveWindow`, of a Claude Code transcript or a Codex rollout whose working directory is in the worktree or whose tool call names its path. A Codex rollout's working directory is its `session_meta`'s and then each `turn_context`'s, and its session the root thread's. A tool call's output does not count.

A session's interactive-mode file (`.pm/run/sessions/<id>`, [Owner-request Stop hook](owner-request-hook.md)) uses a longer window: the session's end removes it (`pm hook session-end`), and `pm clean` lists each one and removes it with `--apply` only once the session's transcripts have been untouched for a day (`ModeWindow`), so a session the owner stepped away from keeps its mode.

### Display

A sprint's holders are derived from its open tasks, not stored. `pm show <id>` shows a task's holder with its liveness, `pm show --project` lists each sprint's holders, and plain `pm show` warns about tasks other live sessions hold, marked `held by <sid>, <age>, live|idle`.

### Known limits

- A live session on another machine looks idle, so its claim can be taken over.

## Alternatives considered

> What else was considered and not adopted, and why not?

| Alternative | Why not |
|---|---|
| A lock file per task | The claim is already one compare-and-set write; a lock adds stale-lock recovery for nothing |
| `--force` to take a live claim | Invites taking work from a running session; an idle claim is taken over anyway |
| `CODEX_THREAD_ID` as a Codex session's id | A subagent's thread has its own id, so it could not act on what its session holds |
| `CLAUDE_CODE_SESSION_ID` first whenever set | A Codex run started from Claude Code would hold and answer for the Claude Code session |
| Taking either id when both are set and no runtime is an ancestor | Either could be this command's; a wrong guess holds or answers for another session |
| Walking every rollout for liveness | Cost grows with the user's whole Codex history |

## Prior art

> Optional. What existing work did we learn from, and what did we take?

None yet.

## Open questions

> What is still unresolved?

- Cross-machine liveness: two machines working at once would need a liveness signal stored in the work store.
