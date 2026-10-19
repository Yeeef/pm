A closed project or sprint can be reopened, keeping its history in one record.

### Added

- `pm project reopen <name>` and `pm sprint reopen <id>` set a closed project or sprint open again. Each takes the
  reason as its body, on two lines or more, and records it, with the close reason it had, as a decision in the
  reopened record (`--confirmed` marks it source=owner). The record's Outcome, or the sprint's delivery report, goes
  back to "Not closed yet.", so the next close needs a fresh one; the old text stays in git history. A sprint of a
  closed project is refused until `pm project reopen` reopens the project.
