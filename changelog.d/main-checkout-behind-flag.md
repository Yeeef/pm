### Added

- The site (home and project pages), `pm service status` and `pm show` flag a main checkout that is behind
  `<remote>/<main branch>` as of the last fetch, with the commit count and the command that fixes it
  (`git -C <main checkout> pull --ff-only <remote> <main branch>`), and one that is on another branch or a detached
  HEAD, with `git -C <main checkout> switch <main branch>`. Hooks, rules and links read the main checkout, so a stale
  one ran old pm rules unseen. `pm service status` exits non-zero while the flag is up; `pm show --json` carries it as
  `main_checkout`.
