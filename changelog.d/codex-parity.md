### Upgrade guide

1. `pm upgrade --to X` also rewrites `.codex/hooks.json`: Codex's SessionStart hooks run on `compact` too, and a new
   SubagentStop hook runs `pm hook stop`. Commit it with the pin move; Codex asks once to trust the changed hooks.

### Changed

- A Codex subagent now shares its root thread's session, as a Claude Code subagent shares its session's: pm maps
  `$CODEX_HOME`'s rollout of the subagent's thread to its root thread, so the subagent may claim, release and answer
  what its session holds and raised.
- With both `$CLAUDE_CODE_SESSION_ID` and `$CODEX_THREAD_ID` set (Codex started from a Claude Code session inherits
  the first), the session is that of the nearest Claude Code or Codex process pm runs under; it was always Claude
  Code's.
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
