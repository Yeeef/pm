# Changelog

Every change to Go pm that its users can notice, by release. The format is
[Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/), and pm follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). History before 0.2.0 (Python pm) is in the git tags
only.

- `## [Unreleased]` comes first and holds what is merged but not released. A release renames it to
  `## [X.Y.Z] - YYYY-MM-DD` (the release's date) and opens an empty one above it.
- A release's section opens with a summary paragraph: what the release is about, in one to three sentences.
- Then these `###` categories, only these, in this order, each only when it has an entry: `Breaking changes`,
  `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed`, `Security`.
- An entry is a `- ` bullet written for pm's users: what they can now do, or what behaves differently. Wrap lines
  freely: the release notes join them.
- Every `Breaking changes` bullet has a sub-bullet `- Upgrade: …` that says what users must do.
- Each section has a link reference at the bottom: its compare view against the release before it.

`release/changelog.py check` checks these rules; the release workflow publishes each release's section as its notes.

## [Unreleased]

### Changed

- Each GitHub release's notes are its section of this changelog, by category, with upgrade steps for every breaking
  change; a release tag without a section fails to release.

## [0.3.0] - 2026-10-09

The pm service now holds each clone's work store, and every `pm` command reaches the store through it. Loading the
store takes about 6 ms instead of 36 ms, and concurrent writers no longer queue on a file lock: 8 processes making 20
writes each finish in about 2 s instead of 7 s (both measured on a 590-item store).

### Breaking changes

- Every command that reads or writes the work store needs the clone's pm service running, on the same pm version.
  With the service down, the command fails with "the pm service does not answer on …/work.sock … run pm service
  restart"; with the service on another version, it fails naming the version each runs.
  - Upgrade: after moving the repo's pin (`pm upgrade --to 0.3.0`), run `pm service restart` in each clone. A session
    start starts a service that is down, but never restarts a running one.
- The work store's schema moves to version 3 the first time 0.3.0 opens it, and the store syncs through the remote,
  so a clone still on 0.2.x then fails its sync with "the remote's schema is version 3, newer than this pm's 2".
  - Upgrade: move every clone of a repo to 0.3.0 together.

### Changed

- Writes queue in order of arrival under one lock that the service holds, from a write's first read to its commit. When
  the holder's connection drops, the next writer takes the lock within 50 ms; a writer that waits 60 s fails, naming
  the process that holds the lock and its write.
- Syncs, creates and the store's setup run inside the service with time limits (120 s, 180 s and 300 s); a limit
  stops only a fetch, push or clone, never a local merge partway.
- `pm init` starts the pm service before it sets up the work store, and session start starts an installed service
  that is down.

### Removed

- The per-command store lock and its log, `.pm/run/work-gate.log`.

### Fixed

- A create whose push timed out but later landed could be made twice; now it fails, naming the id the item may have
  landed as.
- The site's refresh no longer makes a concurrent records write (such as `pm design new`) fail with "Unable to create
  index.lock".

## [0.2.2] - 2026-10-09

A fix for repos that retired Beads by replacing its `.beads/` directory with a file.

### Fixed

- `pm doctor`, `pm upgrade` and `pm init` no longer fail with "open …/.beads/hooks/post-checkout: not a directory"
  when `.beads` is a regular file; it counts as no Beads, as a missing `.beads/` does.

## [0.2.1] - 2026-10-09

Go pm's texts now describe Go pm: they name the work store and the installed pm instead of Beads and the pm uv tool.
pm is licensed under MIT.

### Added

- The MIT license.

### Changed

- `.pm/README.md` says the work store holds the work, and installs pm with the release's `install.sh` instead of
  `uv tool install`. `pm doctor` reports it changed until `pm upgrade` rewrites it.
- The Codex SubagentStart hook's status message reads "Loading pm's git rule for agents".
- Help texts (`pm service`, `pm upgrade --to`, `pm uninstall`), the site's Decisions, Actions and day pages, `pm
  check`'s errors, new records' Progress prompts and the need refusals name the work store, not Beads.

## [0.2.0] - 2026-10-09

The first release of Go pm, from its own public repo, Yeeef/pm. pm is now one binary that holds the work in its own
work store instead of Beads, and installs and downloads releases with no token.

### Breaking changes

- pm is a binary installed with the release's `install.sh`, not a uv tool, and it comes from Yeeef/pm, not
  Yeeef/yeeef-agents.
  - Upgrade: install it on each machine with
    `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.2.0/install.sh | sh`.
- The work lives in pm's work store (embedded Dolt, at `<main checkout>/.pm/store/work`, synced through the remote's
  `refs/pm/work`) instead of Beads; pm runs no `bd`. `pm init` refuses a repo whose remote holds Beads data but no
  work store.
  - Upgrade: in one clone, export the work (`bd export > FILE`), import it (`pm init --import-bd FILE`), then run
    `pm init`, which pushes the store; then run `pm init` in every other clone.
- `pm init` and `pm upgrade` take Beads' pieces out of the repo: bd's hook entries in `.claude/settings.json` and
  `.codex/hooks.json` and the Beads block in `CLAUDE.md`. pm's git hooks move to the tracked `.pm/hooks`, and
  `core.hooksPath` moves there from `.beads/hooks`; `.beads/` itself stays on disk. `pm prime --subagent` prints pm's
  commit-and-push rule instead of the Beads agent profile.
  - Upgrade: run `pm upgrade --to 0.2.0` in the repo and commit what it changes; `pm doctor` names any Beads piece
    left.

### Added

- `install.sh`, a release asset: it checks the binary against the release's `SHA256SUMS` and installs it to
  `${PM_BIN_DIR:-$HOME/.local/bin}/pm`.
- One `pm` on PATH runs each repo's pinned version: a Go pin's release binary, downloaded once and checked against
  its `SHA256SUMS` and the sha256 kept from its first download; a Python pin (0.1.x) through `uv tool run`.
- Work-store commands: `pm task ready`, `pm show <id>`, `pm task add --parent <task>`, `pm task edit`,
  `pm task release`, `pm dep add`, `pm dep rm`, `pm comment add`, `pm need dismiss`, `pm reply add`, `pm sync`, and
  `pm export`, which prints the work store.
- `pm init --import-bd FILE` imports a `bd export` into the work store.
- `pm version` prints the binary's version.

### Changed

- Releases download from Yeeef/pm with no token. Only when that download fails does pm retry through the GitHub API
  with a token from `$GH_TOKEN` or `gh auth token`; `$PM_RELEASE_URL` names a mirror.
- `pm prime`'s rules name the work store and pm's commands instead of `bd`; `bd remember` is gone.

[Unreleased]: https://github.com/Yeeef/pm/compare/pm-v0.3.0...HEAD
[0.3.0]: https://github.com/Yeeef/pm/compare/pm-v0.2.2...pm-v0.3.0
[0.2.2]: https://github.com/Yeeef/pm/compare/pm-v0.2.1...pm-v0.2.2
[0.2.1]: https://github.com/Yeeef/pm/compare/pm-v0.2.0...pm-v0.2.1
[0.2.0]: https://github.com/Yeeef/pm/compare/pm-v0.1.6...pm-v0.2.0
