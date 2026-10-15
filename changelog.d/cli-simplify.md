pm's commands cost agents fewer retries: `pm show --record` alone prints the record's outline, the need forms list
every broken rule at once, every `--help` comes from one renderer, and `pm --help` lists only what agents run. The
rules `pm prime` prints are shorter.

### Breaking changes

- `pm task close <id>` without `--reason` is refused; it recorded "Done", which says nothing about the work. The
  refusal says what the reason holds; a task not done still closes with `--dropped --reason "…"`.
- `pm doc new --bead ID` is gone: give `--sprint ID` or `--task ID` (or `--project NAME`, as before). Each is checked
  for its item type.

### Added

- `pm show --record X` without `--section` prints the record's outline: its header fields, then each section with
  its line count and the `pm show --record X --section <name>` command that prints it.
- `--record`, and `pm record link`, also take a sprint's name (`<project>-<n>`, its record's file name) beside its
  id, a project name, a design slug and a path.
- `pm --help --all` lists the machinery commands `pm --help` now leaves out, and `pm service --help --all` lists
  `service run`.

### Changed

- `pm decision need`, `pm action need` and `pm need edit` check every rule, then refuse once, listing each broken
  rule in its own wording; a single broken rule reads as before.
- `pm --help` leaves out what the runtimes' hooks, the scheduler and the pm service run: `hook`, `prime`, `push`,
  `sync`, `export`, `day` and `autopilot`, and `pm service --help` leaves out `service run`. Each still runs and keeps
  its own `--help`; `pm prime`'s noun list is the visible one.
- The `--help` of `need dismiss`, `reply add`, `task ready`, `task edit`, `task release`, `dep add`, `dep rm`,
  `comment add`, `sync`, `show ID` and `task add --parent` has the layout of every other command: the usage line with
  `[-h]`, metavars instead of types, positional arguments listed.
- `pm task add`'s usage line shows `(--sprint ID | --parent TASK)`, and `pm decision add`'s shows `--level` as
  required.
- The `--help` of `decision add`, `decision close` and `action done` says they refuse while the need holds a site
  reply not delivered yet.
- `pm prime`'s rules are shorter.
