---
type: doc
title: Hooks for the pm harness
date: 2026-10-04
bead: yeeef-agents-9va.14.1
---

Which agent hooks the pm harness should use, and which to skip. Hooks are checked against the Claude Code hooks reference [CC] and the Codex 0.131.0 source at tag `rust-v0.131.0` [CX]; costs were measured in this repo on 2026-10-04 (tokens are bytes / 4). Hooks for context size belong to the context-efficiency project and are not covered.

Sources: [CC] https://code.claude.com/docs/en/hooks; [CX] https://github.com/openai/codex at `rust-v0.131.0`, paths under `codex-rs/`, and https://developers.openai.com/codex/hooks; [BD] `bd prime --help`, `bd hooks --help` (bd 1.3.1) and `.beads/hooks/`.

## Hook events

| Event | Claude Code [CC] | Codex 0.131.0 [CX] |
|---|---|---|
| SessionStart | Matchers `startup`, `resume`, `clear`, `compact`, `fork`; stdout or `additionalContext` is injected; exit 2 only shows stderr | Yes; `additionalContext` (`hooks/src/events/session_start.rs`) |
| UserPromptSubmit | `additionalContext` as a system reminder with the prompt; `decision:"block"` drops the prompt; 30 s default timeout | Yes; context or block (`events/user_prompt_submit.rs`) |
| PreToolUse | `permissionDecision` allow, deny, ask, defer; exit 2 blocks; can rewrite input; fires inside subagents with `agent_id`, `agent_type` | Yes; deny and `additionalContext` only, `ask` is an error and the call proceeds; covers shell (as `Bash`) and `apply_patch` (matchers `Edit`, `Write`; input is patch text) (`engine/output_parser.rs:319-441`, `core/src/tools/hook_names.rs:34-38`) |
| PostToolUse | `additionalContext`; cannot undo the call | Yes; block, context (`output_parser.rs:381-389`) |
| Stop / SubagentStop | `decision:"block"` + `reason` makes the agent continue; input has `stop_hook_active` to avoid loops | Stop only; block + reason continues (`events/stop.rs:37-40`). No SubagentStop |
| PreCompact / PostCompact | PreCompact can block compaction, cannot inject context | Common fields only |
| Others | PermissionRequest, Notification, SessionEnd, SubagentStart, PostToolUseFailure, ConfigChange, WorktreeCreate and more | PermissionRequest; legacy `notify` on turn end, no block or context (`hooks/src/legacy_notify.rs`) |

Common limits:
- Claude Code caps `additionalContext` at 10,000 characters. Longer output goes to a file and the agent sees the path and a 2,000-character preview [CC]. Matching hooks run in parallel, and the default command timeout is 10 min [CC].
- Codex hooks are on by default (`features/src/lib.rs:844-848`) and run only `command` handlers. Each non-managed hook runs only after the owner approves its hash in `/hooks` (`engine/discovery.rs:475-499`). Codex reads `hooks.json` or `[hooks]` in `config.toml`, in the same shape as Claude Code (`config/src/hook_config.rs:125-142`).
- Neither `~/.codex/config.toml` nor `~/.codex/hooks.json` sets a hook here, so Codex sessions in this repo get no `bd prime` today.

## Prior art: Beads

- `bd prime --hook-json` wraps its context in the SessionStart JSON envelope for Claude Code, Gemini CLI and Codex. Its stated purpose is "to prevent agents from forgetting bd workflow after context compaction" [BD]. It can be overridden with `.beads/PRIME.md` and capped with `--max-memories` [BD].
- It runs at SessionStart with an empty matcher (`.claude/settings.json`), so it fires again after `compact` and `clear`. That re-injection is what a PreCompact hook would otherwise be needed for.
- Beads checks git-side invariants with git hooks rather than agent hooks: `pre-commit`, `post-merge`, `pre-push`, `post-checkout` and `prepare-commit-msg`, each a thin shim with a timeout (`.beads/hooks/pre-commit`). The records guard already rides in that pre-commit hook, backed by CI (`records/design/records-store.md`, Guard).
- Lesson: put context in at SessionStart, and enforce invariants where every agent and every tool must pass (git hooks, `make render`), not in per-tool agent hooks.

## Measured costs

| Command | Wall time (3 runs) | Output | Tokens |
|---|---|---|---|
| `bd prime --hook-json` | 0.25 s warm, 0.54 s cold | 6,923 B | ~1,730 |
| `bin/pm show` | 0.81 s | 2,433 B | ~610 |
| `bin/pm show --json` | 0.82 s | 5,801 B | ~1,450 |
| `bd list --status in_progress --assignee <me> --json` | 0.50 s | 3,548 B | (check only) |
| `bd list --label human --status open --json` | 0.46 s | 3 B | (check only) |
| `git -C .records status --porcelain` | <0.01 s | 0 B | (check only) |

## Candidate hooks

| # | Hook | Event | What it does | Benefit | Cost | CC | Codex |
|---|---|---|---|---|---|---|---|
| 1 | Session context | SessionStart, all matchers | Inject `pm show` (text) next to `bd prime` | The agent starts, and restarts after compaction, knowing the projects, sprints, its in-progress tasks, open needs and recent decisions; RULES.md says to start there | +0.8 s per start; ~610 tokens; grows with open sprints, but stays far under the 10,000-character cap; if `pm` fails, the agent just lacks the context (exit non-zero shows an error, nothing blocks) | Yes | Yes (needs trust approval once) |
| 2 | Uncommitted records | Stop | Block once, with a reason, when `.records` has uncommitted changes; skip when `stop_hook_active` | Hand edits to Goal, Done when, design pages and delivery reports get committed before the agent hands back; `pm` writes already commit themselves | <0.01 s per stop; 0 tokens when clean; when wrong (an edit left on purpose) it costs one extra turn, and `stop_hook_active` prevents a loop | Yes | Yes |
| 3 | Unclosed claimed task | Stop / SubagentStop | Remind or block when a task claimed by this agent is still open | Tasks get closed with `pm task close` | 0.5 s per stop. **Wrong often:** Stop fires at the end of every turn, and a task stays open across turns by design. Every agent claims as the same git user, so a subagent cannot tell its own task from another's. No SubagentStop in Codex 0.131.0 | Yes | Stop only |
| 4 | Generated sections | PreToolUse on Edit/Write | Deny edits that touch Progress, Needs you, or a day's Sprints | Keeps generated sections clean | Parsing edit and patch input per tool call. Bypassed by Bash and `sed`. A `make render` check catches the same mistake from any tool and any agent, and it already runs before every records commit | Yes | Partly (patch text) |
| 5 | Records on code branches | PreToolUse on Edit/Write | Deny writes under `records/` on a code branch | None left: `records/` is a link to the store, so edits land on the `records` branch, and the pre-commit guard plus CI already fail a code-branch commit that edits `records/` | Per-call latency and false denials | Yes | Partly |
| 6 | Preserve pm state | PreCompact | Save pm state before compaction | None beyond #1: PreCompact cannot inject context [CC], and SessionStart `compact` re-injects | Extra hook for no gain | Block only | No context |
| 7 | Owner needs | UserPromptSubmit | Show new needs or owner actions with each prompt | The owner sees what waits on them | ~0.5 s on every prompt, plus tokens on each prompt when non-empty. The owner is the one typing and already sees needs on the site and in `pm show` at session start | Yes | Yes |

## Recommendation

Build first:
- **#1 Session context.** SessionStart runs `pm show` alongside `bd prime`, in Claude Code and in Codex (Codex needs `bd prime` added too). It is the cheapest hook with a clear gain, and the same event covers what #6 would.
- **#2 Uncommitted records.** A Stop hook that blocks once when `.records` is dirty. It is cheap and rarely wrong, and it works in both runtimes.

Do instead of a hook:
- **For #4:** make `make render` fail when a generated section holds hand-written text. Today `harness/site.py` inserts the generated content after the prompt lines and keeps whatever text sits below it.

Skip:
- #3, because it would be wrong at most turn ends.
- #5, because the git guard already covers it.
- #6, because SessionStart `compact` already covers it.
- #7: the per-prompt cost is not worth it while needs show at session start and on the site. Revisit it if the owner misses needs in practice.

Where they live: project hooks in `.claude/settings.json` and `.codex/hooks.json`, installed by `pm setup`. A plugin `hooks/hooks.json` is the alternative if the harness ships as a plugin [CC][CX].
