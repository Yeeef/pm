Agents can now give feedback on the repo itself, its code, docs and tools, answering a starting set of questions.

### Added

- `pm feedback add --repo` appends an entry, tagged as `pm feedback add` tags one, to the repo's repo feedback doc,
  `records/docs/repo-feedback.md`, created on first use; the pm feedback doc is unchanged.
- `pm feedback questions` prints the questions an agent answers there, once per sprint when its delivery report is
  written, as the rules now say.
- `pm show` and `pm show --project` print a `repo feedback:` line, and `pm show --json` a `repo_feedback` field,
  beside the pm feedback ones.

### Changed

- A doc titled `repo feedback` anywhere but `records/docs/repo-feedback.md` now fails `pm check`, as a second
  `pm feedback` doc does: move its entries into that doc.
