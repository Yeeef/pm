The site shows each record as published on the `records` branch, and every draft of it beside it as a version.

### Added

- The store's uncommitted record edits are a draft, `store`. A page that a draft changes carries a version bar; the
  draft's version renders in full at `/drafts/store/<record>.html`, its links staying in the version, and its diff
  against the published text, section by section, at `/drafts/store/<record>.diff.html` (`?vs=<draft>` diffs two
  drafts). A draft that does not render fails only its own version's pages; its diff still shows.

### Changed

- The site renders the records committed on the `records` branch, not the store's working tree: a hand edit no
  longer shows as published before `pm commit`, and a record that does not parse no longer turns every page into an
  error.
