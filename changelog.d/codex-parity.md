### Upgrade guide

1. `pm upgrade --to X` also rewrites `.claude/settings.json` and `.codex/hooks.json` (a new `pm hook inbox` on
   UserPromptSubmit; in Codex, SessionStart on `compact` too and a SubagentStop hook) and writes
   `.codex/rules/pm.rules`. Commit them with the pin move.
2. Where Codex works in the repo: open Codex in the main checkout, trust the project if it is not, run `/hooks` and
   trust pm's hooks, new and changed ones included. `pm doctor` names any of pm's hooks Codex would skip.
3. For owner replies to reach Codex sessions, have codex 0.149.0 or later on the pm service's PATH: run
   `pm service install` from a shell where it is. `pm doctor` names a missing or older one.

### Added

- The pm service delivers an owner reply into a live Codex session as it pushes one into a Claude Code session: with
  `codex queue` under the CODEX_HOME the session raised the need with, which starts a turn when the session is idle
  (measured: 6.1 s after the reply in an idle Codex TUI). A Codex session's need records that CODEX_HOME; a session
  that has ended keeps the reply undelivered, and a codex missing from the service's PATH or older than 0.149.0 is
  named on the reply's card and by `pm doctor`.
- `pm reply wait <id> [--timeout DURATION]`: wait within the turn for a new reply from the owner to a need, then
  print it (and mark it delivered, as `pm reply read` does); a reply delivered before the wait is no new one, so an
  agent that asks again waits for the next. The output of a raised need names it.
- `pm hook inbox`, on UserPromptSubmit in Claude Code and Codex: hands a session the owner's replies that did not
  reach it as context beside its next prompt, and marks them delivered once printed.
- `pm init` writes `.codex/rules/pm.rules`, which runs `pm` outside Codex's sandbox: a pm command reaches the pm
  service through a socket that a network-off sandbox blocks. A command Codex cannot split into plain commands, such
  as one fed a heredoc, still runs inside it.
- `pm doctor` asks Codex (`codex app-server`) which of pm's hooks it would skip, untrusted, changed since trusted or
  disabled, or whether it runs none of the project's, and names them with the fix; a typed `pm init` names them too.
  pm never writes Codex's trust. Without `codex` on PATH, or without `$CODEX_HOME` (Codex never ran on the machine,
  and the call would make its state), it says the trust was not checked.

### Changed

- A Codex subagent now shares its root thread's session, as a Claude Code subagent shares its session's: pm takes
  Codex's `$CODEX_SESSION_ID`, the root session's id, and with an earlier Codex that sets only `$CODEX_THREAD_ID`,
  maps the thread to its root through its rollout under `$CODEX_HOME`. So a subagent may claim, release and answer
  what its session holds and raised.
- With both Claude Code's and Codex's session set (Codex started from a Claude Code session inherits
  `$CLAUDE_CODE_SESSION_ID`), the session is that of the nearest Claude Code or Codex process pm runs under; it was
  always Claude Code's. With neither among pm's ancestors, pm refuses, naming both ids and which to unset.
- `pm task claim`'s refusal in the main checkout says its git commands make a worktree in any runtime.

### Fixed

- A Codex run started from a Claude Code session no longer stores that session's inbox
  (`$CLAUDE_CODE_MESSAGING_SOCKET`, which Codex inherits) on its needs, so its replies are no longer pushed into the
  Claude Code session; its needs carry its own CODEX_HOME.
- The pm service pushes a reply `pm reply add` stored as soon as the work store moves; it waited up to a minute for
  its next sweep.
- A need closed by `pm decision add --need` or `pm decision close` records the closing session, as other closes do.
- A Codex session's needs trade the Claude Code inbox an earlier pm stored on them for the session's CODEX_HOME, at
  its next session start.
- The site's reply to a need of a live session with no inbox says the reply waits for that session, at its next
  prompt or through `pm reply wait`; it said the session was not running. "Not running" is kept for a session whose
  transcript has ended.
- A typed `pm init` keeps an installed service unit's PATH: run from Codex, which puts its own directory first on
  every command's PATH, it rewrote the unit and restarted the clone's pm service under every live session (their pm
  commands failed meanwhile), and a Claude Code session's `pm init` flipped it back. `pm service install` takes the
  PATH it runs with.

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
