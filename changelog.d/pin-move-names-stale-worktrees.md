### Added

- `pm upgrade` lists the clone's other worktrees whose checked-out branch pins another pm than the one it moves to,
  each with "merge or rebase it onto the pin move": once the main checkout pins the new version and the pm service
  runs it, pm refuses every work-store command in them.

### Changed

- In a checkout whose pin is not the version the clone's pm service runs, the refusal (and `pm where`'s work line,
  `pm doctor`'s work store line and `pm init`'s refusal) names which side of the pin move the branch is on and the fix:
  a branch from before the move merges or rebases onto it (`git rebase <remote>/<main branch>`); the branch that moves
  the pin waits for its merge. `pm where`'s service line says the service runs the main checkout's pin instead of
  calling it stale and naming `pm service restart`, and `pm doctor` no longer reports such a service as stale.
