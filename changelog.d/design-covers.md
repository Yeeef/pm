### Added

- A design page can name the code it is about in its header: `covers: [internal/records/**, prime.md]`, path
  patterns relative to the repo root (`*` within one path segment, `**` as a whole segment for any depth). `pm check`
  fails on a pattern that matches no tracked file.
- `pm action need --pr` and `pm sprint close` refuse a PR that changes a file a design page covers until one of the
  PR's sprints either updated the page (committed it since the sprint opened and links it in its Design pages) or
  recorded why it stays as it is with the new `pm design unchanged <slug> --sprint ID --reason "…"`, a sprint
  decision. They ask GitHub (`gh api`) for the PR's files only when some page has `covers:`, a renamed file's old path included. The review's text links each page
  covering the changed code.
- `design_sources` in `.pm/config.toml`, a list of path patterns: `pm check` lists the tracked source files it matches
  that no design page covers.
- A design page on the site says when the code it covers changed on the remote's main branch (as the clone last
  fetched it) after its last edit: how many commits, and the latest one's date. The merge of a sprint that updated
  or waived the page does not count. A clone that never fetched the remote's main shows "Code changes not checked"
  instead.
