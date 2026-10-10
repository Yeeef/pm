---
type: doc
title: How two sessions duplicated sprint 21
date: 2026-10-05
bead: yeeef-agents-9va.27.1
---

How the main session started a second build of sprint 21 while another session held it, on 2026-10-05, and what would stop it recurring. Times are UTC, from the Claude transcripts of sessions 462bd145 and 86524d12 and `bd show --json yeeef-agents-9va.25.1`.

## What happened

- 14:33:40–47 sprint 21 (9va.25) and its task 9va.25.1 are created.
- 14:35:22 the owner opens session 462bd145 ("work on … sprint 21"). 14:35:39 it runs `bd update yeeef-agents-9va.25.1 --claim`; Beads records `assignee: yeeef`, `started_at 14:35:41`, `lease_expires_at 14:40:41` (a 5-minute lease). 14:35:50 it delegates the build to a worktree subagent (branch `scoped-bd-reads`).
- 14:40:42 it raises need 9va.25.2 under the sprint; the owner answers at 14:48 (closed 14:48:41). 14:51:51 it raises 9va.25.3; answered 14:53:14. Its subagent keeps working; nothing it holds is visible beyond the claim.
- 14:51:55 the main session 86524d12, after closing sprints 14/16/20, delegates "Build pm-harness sprint 21" to subagent a981aa41. Its brief read neither the task's assignee nor the open needs. The subagent claims 9va.25.1: `--claim` is idempotent for the same assignee "yeeef", so it succeeds silently on a claim whose lease had expired 11 minutes earlier.
- 14:54 the main session even raises need 9va.25.4 under the same sprint.
- 14:57:55 the main session lists the sprint's tasks, 14:58:02 inspects 25.2/25.3 and the worktrees, sees the other session's branch, and at 14:58:09 tells its subagent to stop and messages session "pm: sprint 21". About 6 minutes of duplicate work: uncommitted edits (75 lines added, 37 removed), reverted; nothing committed or pushed.

## Root causes

1. Claims carry no session. Every session and subagent claims as git user "yeeef", so Beads cannot tell a second session from the first: `--claim` is idempotent for "yourself", and the 5-minute lease (`lease_expires_at`) expires while a session is still working, since nothing heartbeats it.
2. `pm show` hides the holder. It renders a task as `running` (pm.py maps `in_progress` to running) with no assignee, session or age, and a sprint as running whenever it is open.
3. Delegation does not check. Neither RULES.md nor the main session's brief requires reading claims and open needs before handing a sprint to a subagent; the brief told the subagent to claim, which can never fail here.
4. Nothing ties a session to its work. A session id exists (`CLAUDE_CODE_SESSION_ID` in the environment; `session_id` on every Claude Code and Codex hook's stdin), but no `pm` or `bd` write records it.

## Earlier near-misses

Searched the records (`grep -i "another session|two sessions|duplicate"` over sprints, days, postmortems), transcripts (`already claimed by|another session holds`) and Beads. No earlier case of two sessions on the same task. Related: sprint 10's finding that a failed sandboxed write of one session was swept into another session's `pm commit` (fixed in sprint 13 by committing only named files). The same blind spot: the shared store and shared identity make sessions invisible to each other.

## Candidate fixes

| Fix | Prevents | Cost |
|---|---|---|
| A. `pm task claim` wraps `bd update --claim` and stores the session id (from `CLAUDE_CODE_SESSION_ID` / Codex hook input) and a title in the issue's metadata; subagents inherit the parent's id | Makes the holder knowable; precondition for B–D | Small: one command, rule change "claim through pm" |
| B. `pm show` names the holder of each running task and sprint: session id/title, claim age, last heartbeat | The owner and the main agent see at a glance who holds what | Small, render-only, once A exists |
| C. Refuse in `pm task claim` (and warn in a SessionStart hook listing held work) when another live session holds the task. "Live" = that session's transcript (`~/.claude/projects/…/<id>.jsonl`, or Codex's session file) was written in the last N minutes (default 30), or a heartbeat `pm` writes on each command is fresher than N | Stops the duplicate before it starts, without trusting the 5-minute bd lease | Medium: liveness probe for two runtimes, a `--force` for abandoned claims, a two-session live test |
| D. Rule in RULES.md: before delegating a sprint or task, read its holder and open needs (`pm show --sprint`); briefs say "claimed for you" only after a successful claim | Cheap guard on the exact path that failed here | Tiny, but relies on agents following it |
| E. PreToolUse hook that blocks `bd update --claim` outside `pm` | Closes the bypass of A/C | Small; extra hook per Bash call |

## Recommended default

A + B + C + D: record the session on the claim, show it, refuse a claim another live session holds (liveness from transcript mtime, 30 minutes), and the delegation rule. Skip E until a bypass is actually seen.
