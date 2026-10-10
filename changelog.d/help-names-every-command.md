### Added

- `pm finding add` takes the finding with `--text` or `--text-file` too, as every body command does, and refuses a
  text that starts with `--` (a misspelt option such as `--txt=…`), which it used to store as the finding.

### Fixed

- `pm --help`, each `pm <noun> --help` and the noun list `pm prime` injects name every command pm runs:
  `pm dep`, `pm need`, `pm comment`, `pm sync`, `pm export` and `pm version`, and `pm task ready`, `edit` and
  `release` and `pm reply add`, which ran but were listed nowhere (`pm dep --help` said "invalid choice"). `pm show
  --help` names `pm show ID`, `pm task add --help` names `--parent`, and `pm init --help` names `--import-bd` and
  `--import`. Their flags and behaviour are unchanged.
- `pm decision close` on a need the owner never answered, and its `--help`, name `pm need dismiss` for a need that
  became moot.
- `pm task claim --help` says what `--session` does: it is the session recorded, even when the environment names one.
