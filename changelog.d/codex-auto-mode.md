pm sets Codex's auto mode in the repo's `.codex/config.toml`, and pm doctor names a Linux host where Codex's sandbox
cannot start.

### Added

- `pm init` and `pm upgrade` write Codex's auto mode into the repo's tracked `.codex/config.toml`:
  `approval_policy = "on-request"`, `approvals_reviewer = "auto_review"` and `sandbox_mode = "workspace-write"`, so a
  Codex session in a repo it trusts runs commands in the workspace-write sandbox and the automatic reviewer decides
  each approval request. Your other keys in the file stay, and so does a value you give one of these three keys:
  `pm doctor` names only a missing key, `pm upgrade` adds only that, and `pm uninstall` removes only the keys that
  hold pm's values. Your own `~/.codex/config.toml` is left as it was.
- On Linux, where Codex is installed and has run, `pm doctor` runs `codex sandbox -- true` and fails when bubblewrap
  cannot make its namespaces, the case in which every command Codex runs sandboxed, `apply_patch` included, fails.
  Under Ubuntu 24.04's AppArmor restriction on unprivileged user namespaces, its line gives the AppArmor profile for
  bwrap to add, or the sysctl to lift; pm doctor changes neither itself.
