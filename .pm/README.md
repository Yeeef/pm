# pm

This repo uses pm: the work store holds the work, Markdown records hold the context, and pm writes both and serves
them as a site.

- Install the pinned version (`version` in `config.toml`):
  `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v<version>/install.sh | sh`, then run `pm init`
  in each clone.
- The work store is a Dolt database each clone keeps at `.pm/store/work`; pm syncs it through the remote's
  `refs/pm/work`.
- Records live on the `records` branch. Each clone checks it out once at `.pm/store/records`, and each worktree reads
  it through `records/`, a git-ignored link. `records/` on the main branch is a copy a workflow keeps.
- Agents get pm's rules and the project's state from hooks (`pm prime`, `pm hook <name>`); run `pm --help` for the
  commands.
- `store/` and `run/` here are per clone and git-ignored; `config.toml`, this file and `.gitignore` are tracked.
