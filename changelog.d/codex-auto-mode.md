pm sets Codex's auto mode in the repo's `.codex/config.toml`.

### Added

- `pm init` and `pm upgrade` write Codex's auto mode into the repo's tracked `.codex/config.toml`:
  `approval_policy = "on-request"`, `approvals_reviewer = "auto_review"` and `sandbox_mode = "workspace-write"`, so a
  Codex session in a repo it trusts runs commands in the workspace-write sandbox and the automatic reviewer decides
  each approval request. Your other keys in the file stay, and so does a value you give one of these three keys:
  `pm doctor` names only a missing key, `pm upgrade` adds only that, and `pm uninstall` removes only the keys that
  hold pm's values. Your own `~/.codex/config.toml` is left as it was.
