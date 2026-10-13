The site no longer serves a page set missing a record while a records sync, or another git command, is rewriting the
records store.

### Fixed

- The pm service's lock-free read of the records repeats under the records lock when git held the store worktree's
  index lock during it, as a checkout or a rebase does while it rewrites the files. A git that stalled between
  unlinking a record and writing it again left that file missing from the read and from both of its checks, so the
  read passed without it.
