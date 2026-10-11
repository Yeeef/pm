### Upgrade guide

1. `pm upgrade --to X` also rewrites `.claude/settings.json` and `.codex/hooks.json` (a new `pm hook inbox` on
   UserPromptSubmit; in Codex, SessionStart on `compact` too and a SubagentStop hook) and writes
   `.codex/rules/pm.rules`. Commit them with the pin move.
2. Where Codex works in the repo: open Codex in the main checkout, trust the project if it is not, run `/hooks` and
   trust pm's hooks, new and changed ones included. `pm doctor` names any of pm's hooks Codex would skip.

### Added

- `pm reply wait <id> [--timeout DURATION]`: wait for the owner's reply to a need, then print it and mark it
  delivered as `pm reply read` does; for a session the pm service cannot push into, such as a Codex session. A need
  raised without an inbox says so.
- `pm hook inbox`, on UserPromptSubmit in Claude Code and Codex: hands a session the owner's replies that did not
  reach it as context beside its next prompt, and marks them delivered once printed.
- `pm init` writes `.codex/rules/pm.rules`, which runs `pm` outside Codex's sandbox: a pm command reaches the pm
  service through a socket that a network-off sandbox blocks. A command Codex cannot split into plain commands, such
  as one fed a heredoc, still runs inside it.
- `pm doctor` asks Codex (`codex app-server`) which of pm's hooks it would skip, untrusted, changed since trusted or
  disabled, or whether it runs none of the project's, and names them with the fix; a typed `pm init` names them too.
  pm never writes Codex's trust. Without `codex` on PATH it says the trust was not checked.
- `pm doctor` fails when `claude` is not on PATH: the owner-request hook's judge needs it, in Codex too.

### Changed

- A Codex subagent now shares its root thread's session, as a Claude Code subagent shares its session's: pm takes
  Codex's `$CODEX_SESSION_ID`, the root session's id, and with an earlier Codex that sets only `$CODEX_THREAD_ID`,
  maps the thread to its root through its rollout under `$CODEX_HOME`. So a subagent may claim, release and answer
  what its session holds and raised.
- With both Claude Code's and Codex's session set (Codex started from a Claude Code session inherits
  `$CLAUDE_CODE_SESSION_ID`), the session is that of the nearest Claude Code or Codex process pm runs under; it was
  always Claude Code's.
- `pm task claim`'s refusal in the main checkout says its git commands make a worktree in any runtime.

### Fixed

- A compacted Codex session gets pm's rules and state again: Codex's SessionStart hooks ran on startup, resume and
  clear only.
- Records a Codex subagent left uncommitted block it at its stop (Codex's SubagentStop), as a Claude Code subagent's
  block its session's stop; Codex keeps a subagent's prompt from its parent's transcript, so the parent's stop could
  not tell.
- `pm hook owner-request` lets a Codex turn that ended without a message through; it failed the check on Codex's
  null `last_assistant_message`.
- `pm clean` keeps a worktree a live Codex session works in or names in a tool call; it read only Claude Code's
  transcripts.
- `pm reply read` without ids finds a Codex session's requests; it read only `$CLAUDE_CODE_SESSION_ID`.
- `pm doctor` reports pm's hook entries whose matcher an earlier pm wrote; it compared their commands only.
- Finding a Codex thread's transcript reads only the date directories the thread can have started on, not every
  rollout under `$CODEX_HOME/sessions`.
