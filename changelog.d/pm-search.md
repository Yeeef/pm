An agent can now find earlier work on a topic across every project before it frames a sprint: `pm search` ranks the
records, tasks and needs that hold the query's words and points each at its best section.

### Added

- `pm search <term>… [--kind KIND]… [--project NAME] [-n N]` finds the sprint, project, design, doc and postmortem
  records and the tasks and needs (title, description and close reason) that hold any word of the query, ranked by
  BM25 with each record one document, so a record holding more of the words, and rarer ones, ranks higher wherever
  in it they fall. It prints how many hold every word and how many hold some, then one line per hit at its best
  unit (a section, a `::: decision` block, a design page section, or the item): kind, date, state, the address
  `pm show --record` takes or the item id, and a snippet. A word matches its plural; a term with a space, a `/` or a
  `.` must also appear as written. No hit prints one line and exits 0.
- The design-page index that `pm sprint open` and a session's first `pm task claim` in a sprint print ends with the
  line `prior work: pm search <topic>`, the whole index when no page is listed.
- `prime.md`'s open-a-sprint step asks to `pm search` the topic in two or three wordings before writing the frame.
