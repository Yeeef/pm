`pm check` and its `--help` state the sprint-close rule for covers patterns as pm 0.8.6 applies it.

### Changed

- `pm check`'s line for covers patterns waiting on unmerged work, and `pm check --help`, now say that `pm sprint close`
  holds a sprint only to the patterns it added or changed while open, not to every pattern on its pages.
