An agent starting work now sees the design pages of its area: `pm sprint open` and a session's first `pm task claim`
in a sprint list them.

### Added

- `pm sprint open` prints the design pages its project's record lists, one line each: slug, title and link.
- `pm task claim` prints, on the session's first claim in the task's sprint, the design pages the sprint's record and
  then the project's list, without repeats; a task directly under a project gets the project's. A slug that names no
  design page shows as `missing`. `prime.md` says to read the pages the task touches before changing their code.
