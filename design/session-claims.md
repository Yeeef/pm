---
type: design
title: Session claims
project: pm-harness
---

## Problem

> What are we solving, and why now?

Part of the [Work layer: Beads](work-layer.md) design. Several agent sessions work one clone at once. `bd update --claim` records only an assignee name, which is the same for every session of one user, so two sessions could take the same task and neither `pm show` nor the owner could tell who holds what or whether that holder is still running.

## Goals and non-goals

> What must the design achieve, and what does it deliberately leave out?

**Goals:**

- Each claim names the session holding it and when it was taken.
- A claim held by a live session cannot be taken by another session; a claim left by a finished session can.
- `pm show --project` shows who holds each task and whether that session is live.

**Non-goals:**

- Locking: Beads serialises its own writes.
- Coordinating sessions on different machines (see Open questions).

## Constraints and key facts

> Which facts, findings and constraints shaped the design? Only those that
> still hold; sprint records keep the findings as they happened.

- Claude Code exposes the session id as `CLAUDE_CODE_SESSION_ID`, Codex as `CODEX_THREAD_ID`; the SessionStart hook also passes `session_id`.
- A session's transcript is rewritten while it runs: `~/.claude/projects/*/<id>.jsonl` for Claude Code, a rollout under `$CODEX_HOME/sessions` for Codex. Its modification time is the only local liveness signal.
- Subagents share their parent's session id.
- Transcripts are local files, so liveness can only be read on the machine that runs the session.

## Design

> What is it, in its final state? Free `###` subsections. Decisions and plans
> live in the project and sprint records.

### Claiming

`pm task claim <id>` records `claimed_by` (the session id from `CLAUDE_CODE_SESSION_ID`, `CODEX_THREAD_ID` or `--session`) and `claimed_at` in the issue's metadata, alongside Beads' own claim. It fails hard when no session id is available. The same session, its subagents included, may re-claim. There is no `--force`.

### Liveness

A holder is live when its transcript was modified within `LIVE_WINDOW` (30 minutes). A claim held by a live other session is refused; one held by an idle session is taken over, and the output says so.

### Display

A sprint's holder is derived from its in-progress tasks, not stored. `pm show --project` marks each held task `[held by <sid>, <age>, live|idle]`, and plain `pm show` warns about work other live sessions hold. Claims made before this existed show as "without a session".

### Known limits

- A raw `bd update --claim` bypasses the check.
- A live session on another machine looks idle, so its claim can be taken over.

## Alternatives considered

> What else was considered and not adopted, and why not?

| Alternative | Why not |
|---|---|
| A lock file per task | Beads already serialises writes; a lock adds stale-lock recovery for nothing |
| `--force` to take a live claim | Invites taking work from a running session; an idle claim is taken over anyway |
| Wrapping or blocking raw `bd update --claim` (option E) | Not chosen; the harness tells agents to claim through `pm` |

## Prior art

> Optional. What existing work did we learn from, and what did we take?

None yet.

## Open questions

> What is still unresolved?

- Cross-machine liveness: two machines working at once would need a liveness signal stored in Beads (and an auto-pull at session start).
