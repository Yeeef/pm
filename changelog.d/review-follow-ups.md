### Changed

- pm is built with Go 1.26.9 (was 1.26.2).

### Fixed

- The site no longer shows the records with a file missing while a records sync rewrites them: since 0.5.0 a record
  file gone by the time the service read it was left out of that read, and the site served it until the records
  moved again. The service now reads the records again under the records lock, which the sync holds.
- `pm check` and `pm commit` refuse a `.md` link in the records store whose file does not exist, naming the link and
  its target; `pm check` passed over one, and `pm commit` committed it as a record.
- `pm init`'s clone of the work store could fail with "invalid connection" when git is 2.47 or later: the auto
  maintenance a fetch starts ran detached and removed its lock file while the clone read its directory. Every git the
  pm service runs now finishes its maintenance before it returns.
- `pm uninstall` disables a systemd unit that is enabled but not running (one that crashed), which it left starting at
  the next login after its unit file was gone; and a session start can no longer bring the service back between
  `pm uninstall`'s check for unsynced work and the removal of the unit and the store: both take the clone's install
  lock.
- `pm sprint move` run again on a clone whose records had not synced another clone's finished move writes the same
  records step, which the records sync then drops, instead of one dated that clone's day, which stopped the records
  sync on a conflict: the move's decisions carry the move note's date (UTC).
