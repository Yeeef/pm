A question an agent asks with its runtime's ask tool shows on the owner's site too, where the owner can answer it as
well as in the session's chat; the first answer reaches the session.

### Upgrade guide

1. Move every clone of the repo to this release together. The work store's schema moves to version 4 the first time
   this release opens it, and the store syncs through the remote, so a clone still on an older pm fails its sync with
   "the remote's schema is version 4, newer than this pm's 3: run pm upgrade" until its main checkout has the new pin.
2. Where Codex works in the repo, open Codex in the main checkout and trust pm's hooks again in `/hooks`: `pm upgrade`
   adds a PostToolUse entry to `.codex/hooks.json`, and Codex skips a new hook until it is trusted. `pm doctor`
   names any of pm's hooks Codex would skip.

### Added

- A question an agent asks with AskUserQuestion (Claude Code) or `request_user_input_async` (Codex) is raised as a
  new kind of need, a question, under the newest open task the session holds, or under none when it holds none. The
  site shows it in a "Sessions ask you" section above the decisions, with each question's options to pick and an
  "Other" field for the owner's own answer; `pm show` lists it too.
- The owner answers a question on the site or in the session's chat, and the first answer wins. In Claude Code a
  site answer closes the open question dialog with that answer; in Codex the pm service queues it into the session
  with `codex queue`, unless the owner already answered in the chat. A question answered in the chat closes on the
  site.
- `pm hook ask --runtime claude|codex`, which raises the questions. `pm upgrade` writes it into `.claude/settings.json`
  as a PermissionRequest hook on AskUserQuestion (timeout one day, so the owner can answer from the site for that
  long), and into `.codex/hooks.json` as a PostToolUse hook on `request_user_input_async`.
- A question's answer steers the session that asked and records no decision: `pm decision add --need`,
  `pm decision close`, `pm action done` and `pm need edit` refuse a question and name `pm need dismiss`, which closes a
  stale one.

### Changed

- pm's rules choose how an agent asks the owner by what it asks, not by whether the owner is in the chat: a question
  that steers the session (what to do next, confirm a plan, clarify what the owner meant) goes through the runtime's
  ask tool, or as a decision need where there is none (`claude -p`, a subagent); a decision on a project's or sprint's
  outcome is a decision need; a step only the owner can do is an action need. The owner-request hook's block reason
  says the same.
