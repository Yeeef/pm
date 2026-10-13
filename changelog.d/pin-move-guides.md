A pin move that crosses several releases now shows the upgrade guide of each one before it writes anything, and
moving pins no longer leaves every old pm binary on the machine.

### Added

- `pm upgrade --to X` first prints the upgrade guide of every release the move crosses, after the repo's pin up to
  X, oldest first and grouped by release, before it writes anything. pm X carries every earlier release's notes, so
  this needs no network; when a crossed release's guide is missing from them, it fails and writes nothing.
- `pm upgrade` prunes the launcher's pins (`~/.local/share/pm/pins/<X>/pm`, about 113 MB each) that no checkout on
  the machine pins: no worktree of this clone, nor of any clone with a pm service unit. A pruned pin keeps its
  `sha256`, so a later download of it is still checked against the first one. When any of those checkouts cannot
  be read, it prunes none and says why. `pm doctor` names the pins that `pm upgrade` would prune, without failing.
