pm installs no git hook and no longer sets `core.hooksPath`: session start's `pm init --session-start` sets each new
worktree up, as it already did.

### Removed

- pm's git post-checkout hook, which linked a new worktree's `records/` on `git worktree add`, is retired with the
  clone's `core.hooksPath` setting that ran it. `pm upgrade` removes pm's section from `.pm/hooks/post-checkout` (the
  file goes when nothing else is left in it, and `.pm/hooks` when it is empty); `pm init`, at every session start too,
  `pm upgrade` and `pm uninstall` unset `core.hooksPath` when it names `.pm/hooks`. A hooks path naming another
  directory is left alone, and `pm init` no longer refuses one. `pm doctor` names a leftover file or hooks path. A worktree
  made without an agent session gets its `records/` link from `pm init` there. `pm where` drops its hooks line, and
  `pm hook git-post-checkout` does nothing, for the hook files older pins still run.
