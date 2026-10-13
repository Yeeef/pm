`pm sprint close` holds a sprint only to the design-page `covers:` patterns it added or changed, so another session's pending pattern no longer blocks an unrelated sprint's close.

### Fixed

- `pm sprint close` refused any sprint that held a design page with a pending `covers:` pattern, even one another
  sprint added for a file its open PR adds. It now checks against the remote's main only the patterns the page did
  not hold at the last records commit before the sprint opened: those the sprint added or changed, and every pattern
  of a page made during the sprint.
