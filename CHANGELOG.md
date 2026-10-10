# Changelog

Every change to Go pm that its users can notice, by release. The format is
[Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/), and pm follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). History before 0.2.0 (Python pm) is in the git tags
only.

- `## [Unreleased]` comes first and holds what is merged but not released. A release renames it to
  `## [X.Y.Z] - YYYY-MM-DD` (the release's date) and opens an empty one above it.
- A release's section opens with a summary paragraph: what the release is about, in one to three sentences.
- Then `### Upgrade guide`, always: a numbered list (`1. `, `2. `, …) of the exact steps that move a repo from the
  previous release to this one, starting with installing the release and `pm upgrade --to X`. `[Unreleased]` needs
  one too before a release candidate is tagged from it.
- Then these `###` categories, only these, in this order, each only when it has an entry: `Breaking changes`,
  `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed`, `Security`.
- An entry is a `- ` bullet written for pm's users: what they can now do, or what behaves differently. A breaking
  change says what breaks and why, and may point at its step in the guide. Wrap lines freely: the release notes join
  them.
- Each section has a link reference at the bottom: its compare view against the release before it.

`release/changelog.py check` checks these rules; the release workflow publishes each release's section as its notes.

## [Unreleased]

### Upgrade guide

1. On each machine, install the release.
2. In the repo, run `pm upgrade --to <X>`. Besides moving the pin, it deletes `.github/workflows/pm-records-copy.yml`
   and `.github/workflows/pm-records-guard.yml` and pm's section in `.pm/hooks/pre-commit` (the file goes when
   nothing else is in it), and the commit it prints also untracks the main branch's `records/` copy
   (`git rm -r -q --cached --sparse --ignore-unmatch records`; the records stay on the `records` branch, and each worktree's link
   stays). Make that commit and merge it as an ordinary PR.
3. Once the PR is on the main branch, pull it into the main checkout. `pm init` (session start runs it) then turns
   off the sparse checkout an earlier pm set in each worktree. A branch cut before the merge still tracks
   `records/`: merge the main branch into it.
4. Move every clone, and every worktree's branch, to the new pin before anyone runs `pm feedback add` or step 5: an
   earlier pm fails every command that reads the records once `records/docs/pm-feedback.md` exists, since it reads
   that doc's header (no date in its name, no project) as invalid, and the records store syncs to every clone.
5. If the records store holds pm feedback docs from an earlier release (`records/docs/<date>-<project>-feedback.md`,
   titled `pm feedback`; `pm check` names each one), merge them into one `records/docs/pm-feedback.md`: a header
   `type: doc`, `title: pm feedback` and `date:` the oldest doc's date, with no `project:`; then every entry, in time
   order, each with `About project <name>` (with its `sprint` or `task`, if any) as its first line. Delete the old
   docs and commit all of them with `pm commit`.

### Breaking changes

- The main branch no longer carries a copy of `records/`: records live only on the `records` branch, read on the
  site or on that branch on GitHub, and each worktree keeps its git-ignored `records/` link. pm no longer writes the
  workflow that copied the records branch into the main branch, the pull-request guard that failed an edit of
  `records/`, or the pre-commit section that refused one, and no longer sets a sparse checkout in each worktree to
  keep the copy away from the link; each existed only for the copy (step 2). A branch that still tracks `records/`
  gets that copy where its link goes: `pm init`, `pm doctor` and `pm where` say to merge the main branch (step 3).
- A repo keeps one pm feedback doc, `records/docs/pm-feedback.md`. Any other doc titled `pm feedback`, such as the
  per-project docs earlier releases wrote, fails `pm check` and `pm commit`, and `pm feedback add` refuses, until it
  is merged into the one doc (step 5). Feedback about pm is read in one place, whatever project it is about.
- An earlier pm cannot read a records store that holds `records/docs/pm-feedback.md`: move every clone to this
  release first (step 4).

### Added

- `pm clean` lists every worktree of the clone with keep or remove and the reason, and `pm clean --apply` removes
  the agent worktrees under `.claude/worktrees/` that nobody needs: clean, with their branch on `origin/main`
  (squash merges checked with `gh`) or pushed with nothing beyond. It keeps a worktree that a live process locks
  (Claude Code's lock, with its pid and start time) or that a live session used in the last 30 minutes, one with
  uncommitted changes or commits never pushed, the main checkout, the records store and the worktree it runs in.
  It removes with `git worktree remove` (never `--force`) and deletes a branch only once it is merged.

### Changed

- `pm doctor` names each piece an earlier pm wrote for the copy (the two workflows, the `pre-commit` section, the
  sparse checkout, the tracked `records/` copy). `pm upgrade` and `pm uninstall` remove the workflows and the
  section, `pm init` turns the sparse checkout off, and the commit `pm upgrade` prints untracks the copy.
- `pm hook git-pre-commit` does nothing: an earlier pm's `pre-commit` section still runs it until `pm upgrade`
  removes that section, so a commit goes through in the meantime.
- `pm feedback add` appends to `records/docs/pm-feedback.md`, creating it on first use. `--project` is optional: when
  given, it tags the entry, as `--sprint` and `--task` do.
- `pm show` and `pm show --project` both link the feedback doc, with its entry count. In `pm show --json`, the doc is
  the top-level `feedback` (`null` before the first entry); a project no longer has a `feedback` list.
- The site's overview links the feedback doc under Feedback.

### Fixed

- `pm init --help` now says what `--session-start` does with the service: it starts an installed service that does
  not answer, as session start has done since 0.3.0, so a session in a clone always brings its service up.
- The site no longer shows "error: open …: no such file or directory" in place of every page when a records sync
  rewrites record files while the service reads them: a record file, directory or day summary that is gone by the
  time it is read is left out, and the next look reads the files that replaced it.
- The pm service's garbage collection could delete the work store's live journal when a write or a sync raced it,
  losing the writes since and crashing the service at its next collection ("error dropping journal writer during
  UpdateGCGen"). pm now builds with a Dolt that holds the fix (dolthub/dolt#11312).

## [0.4.0] - 2026-10-10

pm's rule on requests to the owner, the owner-request Stop hook's judge and its block message now state one rule:
ask the owner only for what you cannot do yourself, as a need under a task or sprint, or with AskUserQuestion when
the owner is in the chat. The hook no longer exempts requests that no sprint waits on, a condition it never checked.
Python pm is retired: a repo still pinned below 0.2.0 must run `pm upgrade` before any other pm command (Breaking
changes). `pm init --import` moves a project from one clone's work store to another's.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.4.0/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.4.0`, then commit what it changes and merge it, as an ordinary PR. A repo
   still pinned below 0.2.0 needs this step before any other pm command runs there.

### Breaking changes

- pm no longer runs a repo pinned to a Python release (below 0.2.0) through `uv`: every command there fails, naming
  the fix, `pm upgrade --to <X>` with a release from 0.2.0 on (step 2). Python pm is retired, and pm no longer needs
  `uv` on a machine, but for the one-time move of a clone off the pre-package harness, which `pm init` names.

### Added

- `pm init --import FILE` loads a `pm export` file into an empty work store and keeps every item's id, so a project
  moves from one clone's store to another's. It refuses a store that holds items, and a malformed or unknown line
  fails with its line number.

### Changed

- The owner-request Stop hook blocks every request to the owner left only in chat, whether or not a sprint waits on
  it; a clarifying question asked in plain text now blocks too, and a sentence that makes the next step depend on
  something the owner must provide counts as a request. Its block message tells the agent to do the step itself
  first, to raise a need under the task or sprint the request belongs to (opening one if none holds it), or to ask
  with AskUserQuestion when the owner is plainly in the chat.
- `pm prime`'s rules state the request rule in one line, matching the hook.
- Each GitHub release's notes are its section of this changelog: a summary, an upgrade guide, then the changes by
  category. A release tag without a section fails to release.

## [0.3.0] - 2026-10-09

The pm service now holds each clone's work store, and every `pm` command reaches the store through it. Loading the
store takes about 6 ms instead of 36 ms, and concurrent writers no longer queue on a file lock: 8 processes making 20
writes each finish in about 2 s instead of 7 s (both measured on a 590-item store).

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.3.0/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.3.0`, then commit what it changes and merge it, as an ordinary PR.
3. In each clone, once its main checkout has the new pin, run `pm service restart`; until then every work-store
   command refuses and names that command.
4. Do steps 2 and 3 for every clone of the repo together: a clone left on 0.2.x fails its next sync once another
   clone has opened the store with 0.3.0.

### Breaking changes

- Every command that reads or writes the work store needs the clone's pm service running, on the same pm version.
  With the service down, the command fails with "the pm service does not answer on …/work.sock … run pm service
  restart"; with the service on another version, it fails naming the version each runs. A session start starts a
  service that is down, but never restarts a running one (Upgrade guide, step 3).
- The work store's schema moves to version 3 the first time 0.3.0 opens it, and the store syncs through the remote,
  so a clone still on 0.2.x then fails its sync with "the remote's schema is version 3, newer than this pm's 2"
  (Upgrade guide, step 4).

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

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.2.2/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.2.2`, then commit what it changes and merge it, as an ordinary PR.

### Fixed

- `pm doctor`, `pm upgrade` and `pm init` no longer fail with "open …/.beads/hooks/post-checkout: not a directory"
  when `.beads` is a regular file; it counts as no Beads, as a missing `.beads/` does.

## [0.2.1] - 2026-10-09

Go pm's texts now describe Go pm: they name the work store and the installed pm instead of Beads and the pm uv tool.
pm is licensed under MIT.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.2.1/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.2.1`, then commit what it changes and merge it, as an ordinary PR.
   It rewrites `.pm/README.md` and `.codex/hooks.json` to the new texts.

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

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.2.0/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.2.0`, then commit what it changes and merge it, as an ordinary PR. It takes
   Beads' pieces out and moves `core.hooksPath` to `.pm/hooks`; `pm doctor` names any Beads piece left.
3. In one clone, with the new pin checked out, export the work from Beads and import it: `bd export > FILE`, then
   `pm init --import-bd FILE`.
4. In that clone, run `pm init`: it pushes the work store to the remote's `refs/pm/work`.
5. In every other clone, once it has the new pin, run `pm init`: it clones the work store from the remote.

### Breaking changes

- pm is a binary installed with the release's `install.sh`, not a uv tool, and it comes from Yeeef/pm, not
  Yeeef/yeeef-agents (Upgrade guide, step 1).
- The work lives in pm's work store (embedded Dolt, at `<main checkout>/.pm/store/work`, synced through the remote's
  `refs/pm/work`) instead of Beads; pm runs no `bd`. `pm init` refuses a repo whose remote holds Beads data but no
  work store (Upgrade guide, steps 3 to 5).
- `pm init` and `pm upgrade` take Beads' pieces out of the repo: bd's hook entries in `.claude/settings.json` and
  `.codex/hooks.json` and the Beads block in `CLAUDE.md`. pm's git hooks move to the tracked `.pm/hooks`, and
  `core.hooksPath` moves there from `.beads/hooks`; `.beads/` itself stays on disk. `pm prime --subagent` prints pm's
  commit-and-push rule instead of the Beads agent profile (Upgrade guide, step 2).

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

[Unreleased]: https://github.com/Yeeef/pm/compare/pm-v0.4.0...HEAD
[0.4.0]: https://github.com/Yeeef/pm/compare/pm-v0.3.0...pm-v0.4.0
[0.3.0]: https://github.com/Yeeef/pm/compare/pm-v0.2.2...pm-v0.3.0
[0.2.2]: https://github.com/Yeeef/pm/compare/pm-v0.2.1...pm-v0.2.2
[0.2.1]: https://github.com/Yeeef/pm/compare/pm-v0.2.0...pm-v0.2.1
[0.2.0]: https://github.com/Yeeef/pm/compare/pm-v0.1.6...pm-v0.2.0
