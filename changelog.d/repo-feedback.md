Agents can now give feedback on the repo itself, its code, docs and tools.

### Added

- `pm feedback add --repo` appends an entry, tagged as `pm feedback add` tags one, to the repo's repo feedback doc,
  `records/docs/repo-feedback.md`, created on first use; the pm feedback doc is unchanged.
- The rules now ask for one such entry per sprint, when its delivery report is written; `pm feedback add --help`
  lists what it covers.
- `pm show` and `pm show --project` print a `repo feedback:` line, and `pm show --json` a `repo_feedback` field,
  beside the pm feedback ones.

### Changed

- A doc titled `repo feedback` anywhere but `records/docs/repo-feedback.md` now fails `pm check`, as a second
  `pm feedback` doc does: move its entries into that doc.
