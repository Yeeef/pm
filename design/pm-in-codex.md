---
type: design
title: pm in Codex
project: pm-codex
---

## Problem

> What are we solving, and why now?

pm's guarantees were built against Claude Code: session context at start, one holder per task, no records left uncommitted, no owner request left only in chat, owner replies delivered to the session that asked. An owner who runs Codex and Claude Code on one project needs the same guarantees from both, and the two runtimes differ in hook trust, session identity, sandboxing and reply delivery.

## Goals and non-goals

> What must the design achieve, and what does it deliberately leave out?

- Goal: a Codex session and a Claude Code session get the same pm outcomes for the same work, and can work one project at once, each seeing the other's holds.
- Goal: every difference between the runtimes is either closed or named here with its reason.
- Goal: a check that can fail holds the parity: a CI test on both runtimes' hook inputs, and a live eval on real sessions.
- Non-goal: pm writing Codex's hook trust (owner decision on the pm-codex project).
- Non-goal: waking an idle Codex TUI from outside; Codex offers no path to it.

## Constraints and key facts

> Which facts, findings and constraints shaped the design? Only those that
> still hold; sprint records keep the findings as they happened.

Codex 0.154 (codex-rs `rust-v0.154.0`) against Claude Code 2.1.296:

| Area | Claude Code | Codex |
|---|---|---|
| Project hooks | run once the settings file is there | run only once the user trusts the project and each hook (`hooks.state` in the user's `config.toml`, a hash of the entry); `codex exec` skips untrusted hooks silently |
| Hooks file | the worktree's `.claude/settings.json` | the main checkout's `.codex/hooks.json`, in every linked worktree |
| Session id in commands | `CLAUDE_CODE_SESSION_ID`, shared with subagents | `CODEX_SESSION_ID`, the root session, shared with subagents; `CODEX_THREAD_ID` is the running thread |
| Session id in hooks | payload `session_id` | payload `session_id`, the root session |
| Reply push | `CLAUDE_CODE_MESSAGING_SOCKET` | none; a TUI runs an embedded app-server nobody outside reaches |
| Subagent tool inputs | plain in the parent transcript | encrypted in the parent rollout; Codex has a SubagentStop event with `agent_transcript_path` |
| Stop `last_assistant_message` | a string | nullable |
| SessionStart sources | startup, resume, clear, compact | the same |
| Sandbox | pm's paths in `permissions.additionalDirectories` | Linux with the network off denies `connect()` by seccomp, so no sandboxed command reaches `work.sock`; an execpolicy `allow` rule runs a command outside the sandbox when every segment matches; a heredoc script is one unsplit segment |
| Env inheritance | | Codex keeps its launcher's env but `*_TOKEN`, `*_KEY`, `*_SECRET`: a Codex run started from Claude Code sees `CLAUDE_CODE_SESSION_ID` and the messaging socket |

## Design

> What is it, in its final state? Free `###` subsections. Decisions and plans
> live in the project and sprint records.

Each area's design lives on its own page; this page keeps a summary per area, the Codex-only parts (hook trust, the sandbox), the gaps and the checks.

### Areas

| Area | Codex in short | Page |
|---|---|---|
| Setup | `pm init` writes `.codex/hooks.json` (the same commands per event as `.claude/settings.json`, SessionStart on `startup\|resume\|clear\|compact`, plus SubagentStop running `pm hook stop`) and `.codex/rules/pm.rules`; `pm doctor` reports Codex's trust in pm's hooks; a typed `pm init` keeps the service unit's PATH | [pm as an installable product](../design/pm-product.md) |
| Session identity and claims | `CODEX_SESSION_ID`, else `CODEX_THREAD_ID` mapped to its root; the innermost runtime decides when both runtimes' ids are set, and an ambiguous session refuses; liveness and `pm clean` read Codex rollouts | [Session claims](../design/session-claims.md) |
| Owner replies | No push: `pm reply wait` within a turn, `pm hook inbox` at the next prompt, `pm show` and `pm reply read`; a Codex session never stores an inbox | [Reply delivery](../design/reply-delivery.md) |
| Owner-request check | The same Stop hook; a null final reply passes; a failing check is a silent pass in Codex; the judge still runs `claude -p` | [Owner-request Stop hook](../design/owner-request-hook.md) |
| Uncommitted records | The same Stop hook; a subagent's SubagentStop reads its own rollout (`agent_transcript_path`), since its tool inputs are encrypted in the parent's | [Agent lifecycle](../design/agent-lifecycle.md) |

`TestClaudeAndCodexRunTheSameHooks` holds the two hook files to the same commands per event, except the differences one allowlist names with a reason: the matchers' syntax and Codex's SubagentStop.

### Hook trust

pm never writes Codex's trust. `pm doctor` and a typed `pm init` ask `codex app-server` (`hooks/list`, 10 s) which of pm's hooks in the main checkout's `.codex/hooks.json` Codex would skip (untrusted, modified since trusted, or disabled; or the whole project, when Codex lists none of its hooks), and name each with the fix: open Codex in the main checkout and trust pm's hooks in `/hooks`. With no `codex` on PATH, or no `$CODEX_HOME` (the call would create Codex's state there), it says trust was not checked. Session start does not ask: it would cost seconds at every start.

### Sandbox

`.codex/rules/pm.rules` holds `prefix_rule(pattern=["pm"], decision="allow")`: a command whose every segment is a plain `pm …` runs outside the sandbox without approval, as pm's hooks already do. A command that is not (a heredoc body, a `cd x &&` prefix) stays inside; on Linux its connect to `work.sock` is refused, and pm says so and names `--text-file PATH` or `--text="…"`. The agent's own `git commit` in a sandboxed worktree asks for approval, as Codex does for any `.git` write; `writable_roots` lets a sandboxed session edit records ([Shared records store](../design/records-store.md)).

### Gaps against Claude Code

The 12 gaps the sprint 1 gap list found, each with where it stands.

| # | Gap | Status | How |
|---|---|---|---|
| 1 | Codex runs no project hook until the user trusts it, and `codex exec` skips untrusted ones silently | reported; the owner trusts once | `pm doctor` and `pm init` name the pm hooks Codex skips (see Hook trust). pm never writes the trust (owner decision). |
| 2 | A Codex subagent is its own thread, with its own id | closed | `CODEX_SESSION_ID`, the root session ([Session claims](../design/session-claims.md)) |
| 3 | A Codex run started from Claude Code inherits `CLAUDE_CODE_SESSION_ID`, which pm read first | closed | the innermost runtime decides; an ambiguous session refuses |
| 4 | No reply push into a Codex session | partly closed | `pm reply wait` and `pm hook inbox` at the next prompt. An idle TUI is not woken: left, see Open questions |
| 5 | Codex reads `.codex/` from the main checkout, not the worktree | left | Codex behaviour. A branch's hook change applies once main has it. In the README. |
| 6 | The SessionStart matcher lacked `compact` | closed | matcher `startup\|resume\|clear\|compact` |
| 7 | A subagent's records are invisible to the parent's Stop hook (encrypted inputs) | closed | Codex SubagentStop runs `pm hook stop` on `agent_transcript_path` |
| 8 | The judge needs `claude`; Codex treats its failure as a silent pass | partly closed | A null `last_assistant_message` passes; `pm doctor` names a missing `claude`. The judge on Codex's own model is pm-codex sprint 2. |
| 9 | Codex has no AskUserQuestion, which the rule names | left | A Codex agent asks through a need, the rule's other path |
| 10 | `pm clean` did not see live Codex sessions | closed | it reads Codex rollouts |
| 11 | A network-off Linux sandbox denies `connect()` to `work.sock` | closed, not run live | `.codex/rules/pm.rules` (see Sandbox). Bubblewrap cannot start on the owner's host. |
| 12 | Claude-only wording; `live()` walked every rollout | closed | runtime-neutral refusal; rollouts found by the thread id's date |

Gaps 5 and 9, and the idle TUI in gap 4, are left by a pm-codex project decision. Each gap not closed is detailed under Gaps not closed.

### Gaps not closed

Six of the 12 gaps are not closed: hook trust (1), the idle TUI (4), `.codex/` from the main checkout (5), the judge's runtime (8), no ask tool (9), and the sandbox not run live (11).

#### Gap 1: Codex runs no pm hook until the user trusts it

| | |
|---|---|
| What happens | Codex runs none of a project's hooks or rules until the user trusts the project, then each hook entry. Trust is a hash of the entry, kept as `hooks.state` in the user's `~/.codex/config.toml`. An entry whose text changes is untrusted again, for example when a `pm upgrade` rewrites `.codex/hooks.json`. The TUI asks at startup ("Trust all and continue" or "Continue without trusting"). `codex exec` skips untrusted hooks with no warning. |
| Guarantees lost while untrusted | All of them: no rules or state at session start, no Stop checks (uncommitted records, owner requests), no `pm hook inbox`. `.codex/rules/pm.rules` does not load either, so on Linux every pm command stays inside the sandbox and cannot reach `work.sock`. |
| Evidence | 0 of the owner's 43 Codex sessions in yeeef-agents, whose `.codex/hooks.json` pm wrote, received pm context. `~/.codex/config.toml` held no `hooks.state` entries. |
| Why it is open | pm never writes the trust (owner decision on the pm-codex project). The hash is Codex-internal and undocumented, and writing it would bypass Codex's review step. |
| Until then | `pm doctor` and a typed `pm init` ask `codex app-server` which pm hooks Codex skips, and name the fix (see Hook trust). The owner trusts once per repo and machine, and again after a `pm upgrade` that changes a hook. |
| Not covered | A session in an untrusted repo gets no signal: its hooks are exactly what does not run, and session start does not ask Codex. A check inside every pm command (does this Codex thread's rollout hold pm's context?) would warn the agent; it is not built. |
| Closes when | Codex offers a supported way for a tool to request trust for the hooks it installs. Managed hooks, which an admin installs and which skip trust, fit an org rollout, not a per-repo `pm init`. |

#### Gap 4: an idle Codex TUI is not woken by a reply

| | |
|---|---|
| What happens | The owner answers on the site while a Codex TUI session sits idle after its turn. Nothing reaches the session until the user types its next prompt; `pm hook inbox` then hands the reply over as context. A Claude Code session in the same state is woken: the push starts a new turn. |
| Guarantee degraded | A reply is delivered to the session that asked, but at its next prompt, not at once. |
| Why it is open | A Codex TUI runs an embedded app-server unless one already listens on `$CODEX_HOME/app-server-control/app-server-control.sock`; on the owner's machine none does. `codex app-server daemon start` refuses an npm install. `codex exec resume <thread>` fails while the TUI holds the thread ("already has an active writer"). Codex exports no messaging socket. |
| Until then | A session that must wait runs `pm reply wait <id>` within its turn. It returns within about 2 s of the reply, and Codex polls a background command for up to 300 s per poll. `pm show` flags the undelivered reply to any session. |
| Closes when | Codex offers a supported way to post into a running thread: a shared app-server by default, or a per-session inbox. The pm service would then push through it (`turn/start`), as it does into Claude Code's socket. |

#### Gap 5: Codex reads `.codex/` from the main checkout

| | |
|---|---|
| What happens | A Codex session in a linked worktree loads `.codex/hooks.json`, `.codex/rules/` and its trust entries from the main checkout, and ignores the worktree's own copies. Claude Code reads the worktree's `.claude/settings.json`. |
| Guarantee degraded | A branch that changes pm's hooks or rules (a pin move, a `pm upgrade` in a worktree) does not apply to Codex sessions until the main checkout has the change. Until then, a Claude Code session and a Codex session on that branch can run different hook sets. |
| Evidence | In a sibling worktree only the main checkout's hooks fired, and Codex stored the trust for the main checkout's path. |
| Why it is open | Codex behaviour; pm cannot change where Codex looks. |
| Until then | Hook changes reach main through a PR, as every pm piece does. The README says Codex reads `.codex/` from the main checkout. `pm doctor` checks the main checkout's `.codex/hooks.json`. |
| Closes when | Codex reads project config from the session's worktree. |

#### Gap 8: the judge needs `claude`

| | |
|---|---|
| What happens | The owner-request judge runs `claude -p --model claude-haiku-5-5` in both runtimes, and so does `pm day summarize`. Without `claude` installed and logged in, the hook exits 1. Codex treats a failing Stop hook as a silent pass: no block, and nothing in `--json` or the rollout. |
| Guarantee lost | On a machine with only Codex, an owner request left only in chat is never caught. |
| Until then | `pm doctor` names a missing `claude`. The README lists `claude` as needed in Codex setups too. A null `last_assistant_message`, which Codex allows, now passes instead of failing. |
| Closes when | [pm-codex sprint 2](../sprints/pm-codex-2.md) is on main. A Codex session's judge then runs `codex exec` with a small model, named by its hook entry; the day summary runs on a runtime the repo names; and the labelled cases pass on both backends (owner decision, 2026-10-12). |

#### Gap 9: Codex has no ask tool

| | |
|---|---|
| What happens | The rule allows asking the owner with AskUserQuestion when they are plainly in the chat. Codex 0.154 has no such tool in its default mode: a default-mode ask sits behind the `default_mode_request_user_input` feature, which is under development and off. The owner-request block reason still names AskUserQuestion. |
| Guarantee degraded | None is lost: a Codex agent asks through a need, the rule's other path. It cannot ask in-chat, though, even with the owner watching, and a block reason points it at a tool it lacks. |
| Why it is open | Codex behaviour. |
| Until then | Needs. |
| Closes when | Codex ships an ask tool in its default mode. The rule then names it; that is a rule change in its three texts (prime.md, the judge prompt, the block reasons), with labelled cases. |

#### Gap 11: the sandboxed path is not run live

| | |
|---|---|
| What is designed | `.codex/rules/pm.rules` runs a plain `pm …` command outside the sandbox. This rests on codex-rs `exec_policy.rs:440`, which bypasses the sandbox when every segment of a command is allowed. Without the rule, a network-off Linux sandbox denies `connect()` to `work.sock` by seccomp (`landlock.rs:100-117`). |
| Not run live | Bubblewrap cannot start on the owner's Linux host (`bwrap: loopback: Failed RTM_NEWADDR`), so every live run used `danger-full-access`. Five things have not been run: a pm command reaching `work.sock` under `workspace-write` through the rule; the heredoc case being refused with pm's message (the EPERM path; the tests reach the same code through EACCES); records edits through `writable_roots`; the agent's own `git commit` asking for approval; and macOS Seatbelt. |
| Evidence so far | `codex execpolicy check` allows `pm show` and `pm commit -m x records/a.md`, and does not allow `pmx`, `rm -rf x` or a heredoc script (`TestCodexRulesAllowPmAlone`). |
| Closes when | `make test-live-parity` passes with `PM_PARITY_CODEX_SANDBOX` unset on macOS or on a Linux host where bubblewrap starts. |

### Checks

- CI: `TestClaudeAndCodexRunTheSameHooks`; the harness tests that drive each hook with both runtimes' real payloads (`tests/hook_payloads.json`) and assert the same output.
- Live: `make test-live-parity`, real `claude -p` and `codex exec` sessions on a scratch clone, identical assertions per runtime: the solo scenario (context, claim, finding, need, reply, decision, record, task close), the Stop hook, the reply at the next prompt, and both runtimes on one project at once with claims refused across runtimes.

## Alternatives considered

> What else was considered and not adopted, and why not?

| Alternative | Not adopted because |
|---|---|
| pm writes `hooks.state` for its own hooks | the hash is Codex-internal and undocumented, and writing it bypasses Codex's review (owner decision) |
| `[sandbox_workspace_write] network_access = true` | opens the whole network to every sandboxed command, to reach one socket |
| A unix-socket allowlist for `work.sock` | macOS only; Linux's seccomp filter denies `connect()` regardless |

The alternatives for delivering replies into a Codex session (`codex exec resume`, a shared app-server, the inbox at Stop) are on [Reply delivery](../design/reply-delivery.md).

## Prior art

> Optional. What existing work did we learn from, and what did we take?

- [Codex hooks docs](https://learn.chatgpt.com/docs/hooks); codex-rs `rust-v0.154.0`: `hooks/src/lib.rs`, `core/src/exec_policy.rs`, `core/src/exec_env.rs`, `linux-sandbox/src/landlock.rs`.

## Open questions

> What is still unresolved?

- Gaps 1, 4, 5, 8, 9 and 11: see Gaps not closed, each with what would close it.
