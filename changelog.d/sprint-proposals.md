Agents can propose a sprint for something the repo needs outside their task, and the owner browses proposals on the
site.

### Added

- `pm sprint propose <project> --title "…"` records a proposal: an open sprint with its frame and an `## Evidence`
  section, which becomes its first finding. Its output lists the project's other proposals and open sprints, so an
  agent can drop its own as a duplicate.
- A proposal stays out of `pm show` (every level, `--json` too) and `pm task ready`; `pm show --sprint ID` shows it.
  Adding its first task starts it as an ordinary sprint.
- `pm sprint close <id> --dropped --reason "…"` drops a proposal nobody started; `pm sprint reopen` makes it a
  proposal again. It refuses while a need under the proposal is open or a design-page draft is tagged with it.
- The site lists proposals in a collapsed Proposed section on the overview and each project page, shows a dropped one
  among the closed sprints with its reason, and the day page names a sprint proposed, started or dropped.

### Changed

- `pm task move` leaves a move note on the item (`pm show ID` lists it), which dates when the task joined its sprint.
- A sprint page with no tasks says "No tasks yet." in place of an empty chart.
- `pm feedback add` is for pm itself; the rules now send what the repo needs to `pm sprint propose`.
