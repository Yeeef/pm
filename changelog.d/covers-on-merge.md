`pm check` no longer fails for every session while a design page names a file that only an unmerged branch has; `pm sprint close` holds the page to the remote's main instead, once the sprint's PR is merged.

### Changed

- `pm check` lists a design page's `covers:` pattern that matches no tracked file as waiting on unmerged work, and
  passes. A sprint brings its page to the new state before its PR merges, so a page names a file before main has it.
  A malformed pattern, and a `design_sources` pattern that matches no tracked file, still fail.
- `pm sprint close`, with and without `--merged`, fetches the remote's main and refuses while a design page in the
  sprint's Design pages, or one the design-page gate names for its PRs, has a `covers:` pattern that matches no file
  there. The refusal names the page and the pattern: fix the pattern, or merge the PR that adds the file first.
