---
type: design
title: pm in Codex
project: pm-codex
covers:
  - "internal/install/codex.go"
  - "internal/install/pieces.go"
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
| Reply push | `CLAUDE_CODE_MESSAGING_SOCKET` | `codex queue --thread <session UUID> --message <text>` (codex-cli 0.149.0 and later) writes to `$CODEX_HOME`'s thread queue, which the session's app-server, a TUI's embedded one included, dispatches once the thread is idle (10 s poll) |
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
| Owner replies | The pm service queues the reply into a live Codex session with `codex queue` under the CODEX_HOME its need stores, and an idle TUI starts a turn with it; `pm reply wait` within a turn, `pm hook inbox` at the next prompt, `pm show` and `pm reply read` as in Claude Code; a Codex session never stores the Claude Code socket it inherits | [Reply delivery](../design/reply-delivery.md) |
| Owner-request check | The same Stop hook, with `--runtime codex`: the judge runs `codex exec` on `gpt-5.6-luna`, so a Codex session needs no `claude`; a null final reply passes; a failing check is a silent pass in Codex | [Owner-request Stop hook](../design/owner-request-hook.md) |
| Uncommitted records | The same Stop hook; a subagent's SubagentStop reads its own rollout (`agent_transcript_path`), since its tool inputs are encrypted in the parent's | [Agent lifecycle](../design/agent-lifecycle.md) |

`TestClaudeAndCodexRunTheSameHooks` holds the two hook files to the same commands per event, except the differences one allowlist names with a reason: the matchers' syntax and Codex's SubagentStop. It reads each Stop entry's `--runtime` as the entry's own.

### Hook trust

pm never writes Codex's trust. `pm doctor` and a typed `pm init` ask `codex app-server` (`hooks/list`, 10 s) which of pm's hooks in the main checkout's `.codex/hooks.json` Codex would skip (untrusted, modified since trusted, or disabled; or the whole project, when Codex lists none of its hooks), and name each with the fix: open Codex in the main checkout and trust pm's hooks in `/hooks`. With no `codex` on PATH, or no `$CODEX_HOME` (the call would create Codex's state there), it says trust was not checked.

A session sees it too, in two ways:

| Case | What tells the session | Cost |
|---|---|---|
| Codex ran none of pm's hooks (never trusted) | The first pm command the session runs prints one warning on stderr: its root rollout holds no pm rules before the first prompt | one git call and one stat per command, after the first read of the rollout's opening |
| Codex ran some (after a `pm upgrade` rewrote a trusted entry, Codex skips that one as modified) | The state hook of a Codex session (the innermost runtime among pm's ancestors) opens the state with one line naming the skipped hooks and the fix | a `hooks/list` call (a few seconds, bounded at 8 s, beside init and where) only when `.codex/hooks.json` or the `[hooks]` trust table of the user's config changed since the answer `.pm/run/codex-trust.json` keeps; a hash of both otherwise |

The state hook takes the runtime from the process ancestry, not from a flag on its entry: an entry changed in the same upgrade would itself be untrusted and could not report the others.

### Sandbox

`.codex/rules/pm.rules` holds `prefix_rule(pattern=["pm"], decision="allow")`: a command whose every segment is a plain `pm …` runs outside the sandbox without approval, as pm's hooks already do. A command that is not (a heredoc body, a `cd x &&` prefix) stays inside; on Linux its connect to `work.sock` is refused, and pm says so and names `--text-file PATH` or `--text="…"`. The agent's own `git commit` in a sandboxed worktree asks for approval, as Codex does for any `.git` write; `writable_roots` lets a sandboxed session edit records ([Shared records store](../design/records-store.md)).

### Gaps against Claude Code

The 12 gaps the sprint 1 gap list found, each with where it stands. A gap marked closed is closed on the open PRs: [#47](https://github.com/Yeeef/pm/pull/47) (pm-codex sprint 1) and, for gaps 8 and 9, [#56](https://github.com/Yeeef/pm/pull/56) (pm-codex sprint 2). Neither is on main or in a release yet; the released pm (0.7.1) has every gap open.

| # | Gap | Status | How |
|---|---|---|---|
| 1 | Codex runs no project hook until the user trusts it, and `codex exec` skips untrusted ones silently | reported; the owner trusts once | `pm doctor` and `pm init` name the pm hooks Codex skips (see Hook trust), and a pm command in a session whose hooks did not run warns it once. pm never writes the trust (owner decision). |
| 2 | A Codex subagent is its own thread, with its own id | closed | `CODEX_SESSION_ID`, the root session ([Session claims](../design/session-claims.md)) |
| 3 | A Codex run started from Claude Code inherits `CLAUDE_CODE_SESSION_ID`, which pm read first | closed | the innermost runtime decides; an ambiguous session refuses |
| 4 | No reply push into a Codex session | closed | the pm service queues the reply into a live Codex session with `codex queue`; an idle TUI started a turn on it 6.1 s after the reply (a Claude Code TUI 1.0 s, by push) |
| 5 | Codex reads `.codex/` from the main checkout, not the worktree | left | Codex behaviour. A branch's hook change applies once main has it. In the README. |
| 6 | The SessionStart matcher lacked `compact` | closed | matcher `startup\|resume\|clear\|compact` |
| 7 | A subagent's records are invisible to the parent's Stop hook (encrypted inputs) | closed | Codex SubagentStop runs `pm hook stop` on `agent_transcript_path` |
| 8 | The judge needs `claude`; Codex treats its failure as a silent pass | closed (pm-codex sprint 2) | Each session's judge runs on its own runtime, `codex exec` in Codex; `pm day summarize` on the runtime `summary_runtime` names; a null `last_assistant_message` passes |
| 9 | The rule named only Claude Code's AskUserQuestion; Codex's ask tools differ | closed (#56) | The rule's three texts name each runtime's ask tool: AskUserQuestion; Codex's `request_user_input_async` in Default mode (gpt-6 models: a non-blocking pick list, the answer arrives as a new user message) and the blocking `request_user_input` in Plan mode. The owner-request hook reads the stopping turn's ask tool calls from the transcript and gives their questions to the judge as asked ([Owner-request Stop hook](../design/owner-request-hook.md)). |
| 10 | `pm clean` did not see live Codex sessions | closed | it reads Codex rollouts |
| 11 | A network-off Linux sandbox denies `connect()` to `work.sock` | closed, not run live | `.codex/rules/pm.rules` (see Sandbox). Bubblewrap cannot start on the owner's host. |
| 12 | Claude-only wording; `live()` walked every rollout | closed | runtime-neutral refusal; rollouts found by the thread id's date |

Gap 5 is left by a pm-codex project decision. Each gap not closed is detailed under Gaps not closed.

### Gaps not closed

Three of the 12 gaps are not closed: hook trust (1), `.codex/` from the main checkout (5), and the sandbox not run live (11).

#### Gap 1: Codex runs no pm hook until the user trusts it

| | |
|---|---|
| What happens | Codex runs none of a project's hooks or rules until the user trusts the project, then each hook entry. Trust is a hash of the entry, kept as `hooks.state` in the user's `~/.codex/config.toml`. An entry whose text changes is untrusted again, for example when a `pm upgrade` rewrites `.codex/hooks.json`. The TUI asks at startup ("Trust all and continue" or "Continue without trusting"). `codex exec` skips untrusted hooks with no warning. |
| Guarantees lost while untrusted | All of them: no rules or state at session start, no Stop checks (uncommitted records, owner requests), no `pm hook inbox`. `.codex/rules/pm.rules` does not load either, so on Linux every pm command stays inside the sandbox and cannot reach `work.sock`. |
| Evidence | 0 of the owner's 43 Codex sessions in yeeef-agents, whose `.codex/hooks.json` pm wrote, received pm context. `~/.codex/config.toml` held no `hooks.state` entries. |
| Why it is open | pm never writes the trust (owner decision on the pm-codex project). The hash is Codex-internal and undocumented, and writing it would bypass Codex's review step. |
| Until then | `pm doctor` and a typed `pm init` ask `codex app-server` which pm hooks Codex skips, and name the fix (see Hook trust). The owner trusts once per repo and machine, and again after a `pm upgrade` that changes a hook. A session is told (see Hook trust): a pm command warns once when the rollout holds no pm rules before the first prompt (bounded read of the root rollout's opening, the answer kept in `.pm/run/codex-hooks/<session>`), and a session whose start hooks ran opens with a line naming the pm hooks Codex skips. Live: an agent in an untrusted scratch clone ran `pm show` and its rollout holds the warning; a session in a clone whose Stop entries' trust was stale got the line naming both. |
| Not covered | A session that runs no pm command, where Codex runs none of pm's hooks, gets no signal: its hooks are exactly what does not run. |
| Closes when | Codex offers a supported way for a tool to request trust for the hooks it installs. Managed hooks, which an admin installs and which skip trust, fit an org rollout, not a per-repo `pm init`. |

#### Gap 5: Codex reads `.codex/` from the main checkout

| | |
|---|---|
| What happens | A Codex session in a linked worktree loads `.codex/hooks.json`, `.codex/rules/` and its trust entries from the main checkout, and ignores the worktree's own copies. Claude Code reads the worktree's `.claude/settings.json`. |
| Guarantee degraded | A branch that changes pm's hooks or rules (a pin move, a `pm upgrade` in a worktree) does not apply to Codex sessions until the main checkout has the change. Until then, a Claude Code session and a Codex session on that branch can run different hook sets. |
| Evidence | In a sibling worktree only the main checkout's hooks fired, and Codex stored the trust for the main checkout's path. |
| Why it is open | Codex behaviour; pm cannot change where Codex looks. |
| Until then | Hook changes reach main through a PR, as every pm piece does. The README says Codex reads `.codex/` from the main checkout. `pm doctor` checks the main checkout's `.codex/hooks.json`. |
| Closes when | Codex reads project config from the session's worktree. |

#### Gap 11: the sandboxed path is not run live

| | |
|---|---|
| What is designed | `.codex/rules/pm.rules` runs a plain `pm …` command outside the sandbox. This rests on codex-rs `exec_policy.rs:440`, which bypasses the sandbox when every segment of a command is allowed. Without the rule, a network-off Linux sandbox denies `connect()` to `work.sock` by seccomp (`landlock.rs:100-117`). |
| Not run live | Bubblewrap cannot start on the owner's Linux host (`bwrap: loopback: Failed RTM_NEWADDR`), so every live run used `danger-full-access`. Five things have not been run: a pm command reaching `work.sock` under `workspace-write` through the rule; the heredoc case being refused with pm's message (the EPERM path; the tests reach the same code through EACCES); records edits through `writable_roots`; the agent's own `git commit` asking for approval; and macOS Seatbelt. |
| Why it cannot run here | Ubuntu 24.04 restricts unprivileged user namespaces through AppArmor (`kernel.apparmor_restrict_unprivileged_userns=1`). No profile exempts `/usr/bin/bwrap`, the bubblewrap Codex prefers over its bundled one, so bwrap is moved into the `unprivileged_userns` profile and loses every capability: loopback setup fails (`RTM_NEWADDR`), and without `--unshare-net` its uid map write fails. The kernel log holds 3,191 bwrap transitions into that profile. No Codex setting avoids the user namespace. `network_access = true` fails one step earlier. The deprecated `use_legacy_landlock` mode refuses `workspace-write` and does not protect `.git`. |
| Fix (owner action, needs root) | Ubuntu's `bwrap-userns-restrict` profile from `apparmor-profiles`, the default from Ubuntu 25.04 on and assumed by openai/codex#14963. Also remove the stale writable root `yeeef-agents/.beads` from `~/.codex/config.toml`: it is now a plain file, and Codex aborts every `workspace-write` run on it. |
| Evidence so far | `codex execpolicy check` allows `pm show` and `pm commit -m x records/a.md`, and does not allow `pmx`, `rm -rf x` or a heredoc script (`TestCodexRulesAllowPmAlone`). |
| Closes when | `make test-live-parity` passes with `PM_PARITY_CODEX_SANDBOX` unset on macOS or on a Linux host where bubblewrap starts. |

### Checks

- CI: `TestClaudeAndCodexRunTheSameHooks`; the harness tests that drive each hook with both runtimes' real payloads (`tests/hook_payloads.json`) and assert the same output.
- Live: `make test-live-parity`, real `claude -p` and `codex exec` sessions on a scratch clone, identical assertions per runtime: the solo scenario (context, claim, finding, need, reply, decision, record, task close), the Stop hook, the reply at the next prompt, an idle interactive session (each runtime's TUI in a pty) woken by the owner's reply with nothing typed, and both runtimes on one project at once with claims refused across runtimes; and a Codex session with no `claude` on PATH, asking only in chat, blocked by the judge on `codex exec`.
- Live: the parity eval's ask tool scenario, in each runtime's TUI: a multiple-choice question asked with AskUserQuestion or `request_user_input_async`, answered at the keyboard, is not blocked and raises no need.
- Live: `make test-live` runs the owner-request judge's labelled cases on both runtimes ([Owner-request Stop hook](../design/owner-request-hook.md)).

## Alternatives considered

> What else was considered and not adopted, and why not?

| Alternative | Not adopted because |
|---|---|
| pm writes `hooks.state` for its own hooks | the hash is Codex-internal and undocumented, and writing it bypasses Codex's review (owner decision) |
| `[sandbox_workspace_write] network_access = true` | opens the whole network to every sandboxed command, to reach one socket |
| A unix-socket allowlist for `work.sock` | macOS only; Linux's seccomp filter denies `connect()` regardless |

The alternatives for delivering replies into a Codex session (`codex exec resume`, a shared app-server, async hooks, a blocking Stop hook, an MCP long-poll, the inbox at Stop) are on [Reply delivery](../design/reply-delivery.md).

## Prior art

> Optional. What existing work did we learn from, and what did we take?

- [Codex hooks docs](https://learn.chatgpt.com/docs/hooks); codex-rs `rust-v0.154.0`: `hooks/src/lib.rs`, `core/src/exec_policy.rs`, `core/src/exec_env.rs`, `linux-sandbox/src/landlock.rs`.

## Open questions

> What is still unresolved?

- Gaps 1, 5 and 11: see Gaps not closed, each with what would close it.
