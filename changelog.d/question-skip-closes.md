A question skipped with "Chat about this" or in the Claude app no longer stays under "Sessions ask you".

### Fixed

- A Claude Code question the owner skips, with the dialog's "Chat about this" or in the Claude app (Remote Control),
  closes as dismissed when the session's
  turn ends. Before, Claude Code told no pm hook about the skip, and the question stayed open on the site until the
  session's process exited, in one case 18 hours later. Claude Code's Stop hook now runs `pm hook ask` as well, which
  `pm upgrade` adds to `.claude/settings.json`.
