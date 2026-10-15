A design page's edits now reach the published records when its sprint closes, once its code is on main, so the site
never shows a page describing code that has not merged, and two sprints' edits of one page meet at the close.

### Upgrade guide

1. Before moving the pin, check the published design pages against main: `pm check` now fails on a covers pattern
   that matches no file on the remote's main, where it listed it as waiting on unmerged work. Fix each pattern it
   names in a worktree's records workspace, linked from a sprint's Design pages, and close that sprint.

### Breaking changes

- `pm commit` of a design page from a worktree keeps it in the workspace's draft commit, tagged with its sprint, and
  no longer publishes it: `pm sprint close` does.
- `pm check` fails on a published design page's covers pattern that matches no file on the remote's main (fetched
  when its remote-tracking branch lacks one), and in a worktree on a draft page's pattern that matches no file the
  worktree tracks. The list of patterns waiting on unmerged work is gone, and `pm sprint close` no longer holds the
  patterns a sprint added since it opened by records commit dates: it holds the patterns each page it publishes adds.

### Added

- `pm sprint close` publishes every design page drafted for the sprint, from every worktree and from every
  `drafts/` branch whose worktree is gone, in one records commit with the Outcome's merge stamp. It refuses, changing
  nothing, while a draft overlaps what the records branch published since the draft's base or another draft of the
  sprint, naming the commits or the branch and the fix, and while your workspace has uncommitted edits to a page the
  sprint links. Run again after a stop, it finds nothing left to publish. A sprint with no PR publishes the same way.
- `pm sprint close --discard SLUG --reason "…"`, repeatable, drops the sprint's drafts of a page instead of
  publishing them, puts back your workspace's uncommitted edits to it, and records the reason as `pm design
  unchanged` does.
- `pm where` lists each `drafts/` branch whose worktree is gone, with the design pages it still holds for their
  sprints' close.

### Changed

- The design-page gate (`pm action need --pr`, `pm sprint close`) counts a page as updated when a draft of this clone
  holds it for the sprint.
- A draft page keeps its sprint when the page is renamed on the records branch (`pm design rename`): the draft
  follows it to the new path and publishes there.
