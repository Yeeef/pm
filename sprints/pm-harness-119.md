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
- The commands `pm interactive` and `pm autopilot`: they write or remove a mode file under the main checkout's `.pm/run/sessions/<session-id>`, the session from the environment, and refuse with none. An agent runs them when the owner asks; nothing enforces it (owner decision). While the file exists, `pm hook inbox` adds a line saying so to each prompt.
- `pm hook owner-request` reads the hook input's `session_id` and passes the stop through when that session is interactive. Sessions start in autopilot.
- `pm hook stop` (the uncommitted-records check) stays on in both modes.
- Mode files of sessions that are not live: removed by `pm clean`.
- The rule: `prime.md` tells agents to run `pm interactive` when the owner asks for it. The judge prompt and the block reasons stay: the judge does not run in interactive mode, so no labelled case moves.
- Both runtimes: Claude Code and Codex.

**Out:**
- Any change to the uncommitted-records check.
- Syncing the mode across clones (it is local to the clone).

## Done when

> What evidence will show the goal is met?

- `test_owner_request_hook.py`: an interactive session's chat ask passes the stop; the same ask in another session, in an autopilot session, or after `pm autopilot` is blocked.
- `test_pm.py`: `pm interactive` / `pm autopilot` write / remove the session's mode file, and refuse with no session id.
- `test_hooks.py`: `pm hook inbox` adds the interactive-mode line only while the session is interactive, on both runtimes' input.
- `test_clean.py`: `pm clean` removes a dead session's mode file and keeps a live one's.
- Live check, in a scratch clone: a `claude -p` session told to run `pm interactive`, then resumed with a prompt that makes it ask in chat, is not blocked; after `pm autopilot`, the same ask is blocked.
- PR on main.

## Design pages

> Where is the detail?

- [Owner-request Stop hook](../design/owner-request-hook.md): Interactive mode, its commands, mode file and checks; the prompt keyword and the mode read from the transcript as alternatives not chosen (covers `internal/session/mode*.go`, `internal/cli/mode.go`).
- [Agent lifecycle](../design/agent-lifecycle.md): the UserPromptSubmit row of the hooks table (the interactive-mode line), and the Stop check off in interactive mode (covers `prime.md`, `internal/hooks/prime*.go`, `internal/hooks/stop*.go`).
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

::: decision {source=agent date=2026-10-14}
The design page pm-product stays as it is for this sprint's code changes.
prime.md gains one sentence on interactive mode and prime.go only its file map; the page's install and hook model stay true
:::

::: decision {source=agent date=2026-10-14}
The mode is read from the runtime transcript, not a file: the session is interactive when its latest owner prompt that is exactly pm interactive or pm autopilot is pm interactive. The keyword prompt is not blocked, so it lands in the transcript and starts a short turn that acknowledges it.
Only the runtime writes owner prompts into its transcript, so an agent cannot set the mode short of tampering with that file; with no state file, pm clean has nothing to reset and no idle session loses its mode.
:::

::: decision {source=owner date=2026-10-14}
Ship the shell commands pm interactive and pm autopilot, which create and remove the mode file .pm/run/sessions/<session-id> using the session id from the environment; the Stop hook checks that file. This replaces the prompt keyword and the transcript design. prime.md says an agent runs them only when the owner asks; nothing enforces it.
Any file-based mode can be set by an agent whichever way it is meant to be set, so a prompt keyword buys no protection; reading the transcript would protect it but costs a scan per stop (0.1 s for grep over a 410 MB transcript, warm). The owner chose the simplest design.
:::

::: decision {source=agent date=2026-10-14}
The design page records-store stays as it is for this sprint's code changes.
stop.go only gains mainOf, which StoreOf now uses to find the store; where the store is and how it is found stay as the page says
:::

::: decision {source=owner date=2026-10-14}
The request rule leaves prime.md. The session-start state hook (startup, resume, clear, compact) prints the rule for the session mode, and pm interactive and pm autopilot print the rule of the mode they set. The per-prompt inbox line goes.
A static rule in prime.md contradicts interactive mode; printing the rule per mode at session start, after a compaction and at the switch keeps the agent on the rule that applies, without a line on every prompt.
:::

::: decision {source=owner date=2026-10-14}
A SessionEnd hook (Claude Code and Codex) removes the session mode file. pm clean stays as the backstop for sessions that crashed, removing a mode file only when its session has been idle for a day, not the 30 min used for worktrees.
An ended session cleans up after itself structurally; the one-day window keeps pm clean from resetting an interactive session the owner stepped away from.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Review of the first build (mode file under .pm/run/sessions, set by pm hook
  inbox): an agent can switch interactive mode on by itself in three ways:
  piping a forged UserPromptSubmit event into pm hook inbox, touching the mode
  file, or pointing the Stop hook's cwd at a scratch repo holding one.
  Reproduced with .go/pm: the forged event wrote the file, and pm hook
  owner-request then exited 0 on a chat ask without calling the judge. A file
  the agent can write cannot carry a mode only the owner may set.

- Fresh-context review of the final diff (3 commits, shell commands plus mode
  file): no correctness findings. The same session id resolves on both sides
  (session.Current vs. the hook input's session_id, Claude Code and Codex
  root), and the same main checkout from a worktree. A bad id is refused, and
  an unsafe one reads as autopilot. Two judgment calls, both falling back to
  the strict default or visible: a mode file outlives its session until pm
  clean --apply, which the inbox context line makes visible on resume; and pm
  clean --apply resets an interactive session idle over 30 min.

- Live check, scratch clone (agents.Clone, pm 8646707), real claude -p Haiku
  sessions, 1 passed in 59.65 s. Autopilot chat ask: blocked by the Stop hook
  (10.9 s). A session that ran pm interactive got mode file
  .pm/run/sessions/<its id>. Resumed with the same ask: 0 blocks, 3.3 s, and
  the prompt carried the inbox line. After pm autopilot the file was gone and
  the ask was blocked again (16.1 s). A TUI session's !pm interactive created
  the file for that TUI session's own id, so bash-mode commands get
  CLAUDE_CODE_SESSION_ID. The judge being skipped is inferred: the same ask
  was blocked 2 of 2 times in autopilot and 0 of 1 in interactive, with the
  shorter latency.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: `pm interactive` and `pm autopilot` set a session's mode, and the owner-request judge lets an interactive session's asks in chat through (PR #91, pending merge).

- `pm interactive` / `pm autopilot`: create and remove `<main checkout>/.pm/run/sessions/<session-id>`, with the session id from the environment; both refuse without a session.
- `pm hook owner-request`: skips the judge while the session's mode file exists. `pm hook stop` is unchanged.
- `pm hook inbox`: adds one context line to each prompt while the session is interactive.
- `pm clean`: lists mode files; `--apply` removes those of sessions that are not live.
- `prime.md`: one sentence saying an agent runs `pm interactive` when the owner asks. Nothing enforces that, as the owner decided.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Item | Met | Evidence |
|---|---|---|
| `test_owner_request_hook.py` | met | CI `integration` and `light` pass on PR #91; the test counts judge calls: 0 when interactive; another session, autopilot and after `pm autopilot` are blocked |
| `test_pm.py` | met | Same CI run: the commands write and remove the file, and refuse with no session id |
| `test_hooks.py` | met | Same CI run: the inbox line appears only while interactive, on Claude Code and Codex input |
| `test_clean.py` | met | Same CI run: a dead session's mode file is removed, live Claude Code and Codex sessions' files are kept |
| Live check | met | Scratch clone, real `claude -p` Haiku, 1 passed in 59.65 s: blocked in autopilot, 0 blocks after `pm interactive`, blocked again after `pm autopilot`; a TUI `!pm interactive` set the TUI session's own file (Findings) |
| PR on main | not yet | PR #91, CI green, awaiting review |
