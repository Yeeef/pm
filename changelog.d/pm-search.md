An agent can now find earlier work on a topic across every project before it frames a sprint: `pm search` ranks the
record sections, decisions, design sections, tasks and needs that hold every word of the query.

### Added

- `pm search <term>… [--kind KIND]… [--project NAME] [-n N]` finds the sprint, project, doc and postmortem sections,
  the `::: decision` blocks, the design page sections, and the tasks and needs (title, description and close reason)
  that hold every word of the query, ranked by BM25, and prints one line per hit: kind, date, state, the address
  `pm show --record` takes or the item id, and a snippet. A word matches its plural; a term with a space, a `/` or a
  `.` must also appear as written. No hit prints one line and exits 0.
- The design-page index that `pm sprint open` and a session's first `pm task claim` in a sprint print ends with the
  line `prior work: pm search <topic>`, the whole index when no page is listed.
- `prime.md`'s open-a-sprint step asks to `pm search` the topic in two or three wordings before writing the frame.
