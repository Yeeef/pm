A PR review closed before its PR reached main, as a stacked PR's is, can be restamped once the PR's change is
on main, so its sprint can close; and `pm action need --pr` refuses a stacked PR, whose review could never close.

### Added

- `pm action done <review> --reason "merged as <sha>"` restamps a PR review closed without `merged as <sha>`, as when
  its PR merged into a stacked base: once `<sha>` is on the remote's main branch (pm fetches it first, as
  `pm sprint close --merged` does), the review's close reason becomes the new one with the old one kept after it, and
  its sprint can close. Any other closed action is refused as before.

### Changed

- `pm action need --pr` refuses a GitHub PR whose base branch is not the main branch: a stacked PR merges into another
  PR's branch, so its review could never close as merged on main. The refusal names the fix: raise the review on the
  PR that reaches main, or retarget this one (`gh pr edit <url> --base main`). pm asks `gh pr view` for the base, so
  raising a review on a GitHub PR now needs `gh`.
