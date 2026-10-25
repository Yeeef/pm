A project can be renamed, and `pm check` names a project whose record title has drifted from the work store.

### Added

- `pm project edit <name> --title "…"`, with the reason as its body, renames a project, open or closed: its
  work-store title and its record's title header, in one records commit that records the rename as a project
  decision (source=owner with `--confirmed`). Its name, record path and links stay as they are.
- `pm check` lists each project whose record `title:` differs from its work-store title, with the
  `pm project edit` that fixes it. It is a report: neither `pm check` nor `pm commit` fails on it.
