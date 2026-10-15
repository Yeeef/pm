`pm interactive` turns the owner-request check off for one session while the owner chats in it, and `pm autopilot`
turns it back on; the request rule now follows the session's mode.

### Upgrade guide

1. In Codex, trust pm's two new hooks in `/hooks`: `pm prime --request-rule` on SessionStart and `pm hook session-end`
   on SessionEnd, which `pm upgrade` adds after pm's existing entries in `.codex/hooks.json`. Codex skips them until
   you trust them; pm's other hooks keep their trust, and the session-start state names the two while they are
   skipped.

### Added

- `pm interactive` and `pm autopilot` set the mode of the session that runs them, in Claude Code and Codex. In
  interactive mode the owner-request Stop hook lets asks in chat through for that session alone, without running its
  judge, until `pm autopilot`. Sessions start in autopilot. An agent runs them when the owner asks for the mode;
  nothing enforces that. The uncommitted-records Stop check stays on in both modes. The mode lives in
  `<main checkout>/.pm/run/sessions/`, local to the clone.
- `pm hook session-end`, on SessionEnd in both runtimes, removes an ended session's mode. `pm clean` removes a mode
  that a crashed session left once the session has been idle for a day.

### Changed

- The rule for asking the owner moves out of `pm prime`'s rules: a SessionStart hook of its own,
  `pm prime --request-rule`, gives the rule of the session's mode (never leave a request only in chat, in autopilot;
  ask in chat when you need to, in interactive mode) at startup, resume, clear and compact, and `pm interactive` and
  `pm autopilot` print the rule of the mode they set.
