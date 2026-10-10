### Added

- `pm sprint edit ID --title "…"` renames an open sprint: its work-store title, which keeps its `Sprint <n>: `, and
  its record's title, with the reason (the body, two lines or more) recorded as a sprint decision.
- `pm need edit ID` rewrites an open need's body before the owner replies: an action's description with `--text` or
  `--text-file`, or a decision need's parts with the flags `pm decision need` takes, checked the same way. It refuses
  a need that holds a reply, a closed need and a PR review.
- `pm sprint close ID --merged SHA [--pr URL]` closes a sprint whose PR was merged with no review need and stamps
  `Merged as <sha> (PR #N).` into its Outcome, as a close after a review does. The commit must be on the remote's
  main branch, which pm fetches first; a sprint that holds a review is refused.

### Fixed

- `pm commit` takes a path relative to the store, such as `sprints/x.md`, from anywhere, as it takes
  `records/sprints/x.md`.
