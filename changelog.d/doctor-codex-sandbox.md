pm doctor names a Linux host where Codex's sandbox cannot start.

### Added

- On Linux, where Codex is installed and has run, `pm doctor` runs `codex sandbox -- true` and fails when bubblewrap
  cannot make its namespaces, the case in which every command Codex runs sandboxed, `apply_patch` included, fails.
  Under Ubuntu 24.04's AppArmor restriction on unprivileged user namespaces, its line gives the AppArmor profile for
  bwrap to add, or the sysctl to lift; pm doctor changes neither itself.
