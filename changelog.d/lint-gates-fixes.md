### Fixed

- A pm service that finds the work store held by another process names Dolt's own error as the cause, which it
  used to drop.
- `pm uninstall` fails on a service unit it cannot read, instead of reporting no service installed.
- A reply spool line that is JSON but not a spool entry is refused with its line number, instead of read as empty.
- `pm push` reports git's counts as unreadable when they are not numbers, instead of reading them as 0.
