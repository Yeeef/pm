# Changelog

Every change to Go pm that its users can notice, by release. The format is
[Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/), and pm follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). History before 0.2.0 (Python pm) is in the git tags
only. Every merge to main that carries an entry releases; while pm is 0.x, a minor release asks a repo that moves its
pin for a step beyond installing it and `pm upgrade` (or removes something), and a patch release asks for none.

- What is merged but not released is in `changelog.d/`, one file per change, so that two pull requests never edit
  the same lines: `changelog.d/<slug>.md`, the slug naming the change in lowercase letters, digits and `-`. An entry
  file opens with its summary paragraph, then holds `###` categories and their items, as a release's section does
  (`### Fixed`, then its bullets). It names no step by its number: the release renumbers them. `changelog.d/README.md`
  is no entry: it stays, so the directory pm embeds is never empty.
- This file holds released sections only, newest first: `## [X.Y.Z] - YYYY-MM-DD` (the release's date). A release
  writes its section from the entry files and deletes them (`release/changelog.py release X`).
- A release's section opens with a summary paragraph: what the release is about, in one to three sentences; its
  entries' summaries, joined.
- Then `### Upgrade guide`, always: a numbered list (`1. `, `2. `, …) of the exact steps that move a repo from the
  previous release to this one. The release writes the first two, installing the release and `pm upgrade --to X`;
  a change that needs another step adds it in its entry file as `### Upgrade guide`, numbered from `1. `, and the
  release numbers it after them. A release candidate's notes are the section the entry files give the same way.
- Then these `###` categories, only these, in this order, each only when it has an entry: `Breaking changes`,
  `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed`, `Security`.
- An entry is a `- ` bullet written for pm's users: what they can now do, or what behaves differently. A breaking
  change says what breaks and why. Wrap lines freely: the release notes join
  them.
- Each section has a link reference at the bottom: its compare view against the release before it.

`release/changelog.py check` checks these rules, on this file and each entry file; the release workflow publishes
each release's section as its notes.

## [0.15.0] - 2026-10-15

A design page's edits now reach the published records when its sprint closes, once its code is on main, so the site
never shows a page describing code that has not merged, and two sprints' edits of one page meet at the close.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.15.0/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.15.0`, then commit what it changes and merge it, as an ordinary PR.
3. Before moving the pin, check the published design pages against main: `pm check` now fails on a covers pattern
   that matches no file on the remote's main, where it listed it as waiting on unmerged work. Fix each pattern it
   names in a worktree's records workspace, linked from a sprint's Design pages, and close that sprint.

### Breaking changes

- `pm commit` of a design page from a worktree keeps it in the workspace's draft commit, tagged with its sprint, and
  no longer publishes it: `pm sprint close` does.
- `pm check` fails on a published design page's covers pattern that matches no file on the remote's main (fetched
  when its remote-tracking branch lacks one), and in a worktree on a draft page's pattern that matches no file the
  worktree tracks. A published pattern main lacks while a draft of the page no longer holds it is listed as waiting
  on that sprint's close instead. The list of patterns waiting on unmerged work is gone, and `pm sprint close` no longer holds the
  patterns a sprint added since it opened by records commit dates: it holds every pattern of each page it publishes.

### Added

- `pm sprint close` publishes every design page drafted for the sprint, from every worktree and from every
  `drafts/` branch whose worktree is gone, in one records commit with the Outcome's merge stamp. It refuses, changing
  nothing, while a draft overlaps what the records branch published since the draft's base or another draft of the
  sprint, naming the commits or the branch and the fix, and while your workspace has uncommitted edits to a page the
  sprint links. Run again after a stop, it finds nothing left to publish. A sprint with no PR publishes the same way.
- `pm sprint close --discard SLUG --reason "…"`, repeatable, drops the sprint's drafts of a page instead of
  publishing them, puts back your workspace's uncommitted edits to it, and records the reason as `pm design
  unchanged` does; a later run of the close never publishes that page.
- `pm check`, the design-page gate and `pm sprint close` name a `drafts/` branch they cannot read (a commit made by
  hand there) in a warning and carry on; the close refuses only when a commit of that branch names its sprint. The
  close notes each other worktree's uncommitted edit of a page it publishes.
- A follow that leaves a drafted page with conflict markers keeps its sprint, and the sprint's close refuses from any
  worktree, naming the worktree and the page, until it is resolved and committed again. A page's draft in a
  workspace belongs to one sprint: `pm commit` refuses it for another open sprint.
- `pm where` lists each `drafts/` branch whose worktree is gone, with the design pages it still holds for their
  sprints' close.

### Changed

- The design-page gate (`pm action need --pr`, `pm sprint close`) counts a page as updated when a draft of this clone
  holds it for the sprint.
- A draft page keeps its sprint when the page is renamed on the records branch (`pm design rename`): the draft
  follows it to the new path and publishes there.

## [0.14.0] - 2026-10-15

Docs render as written: TeX math is typeset, links between docs resolve by slug, and a docs index lists every doc; `pm
check` refuses a link to a record that does not exist, and JSX in raw HTML. pm does not support MDX.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.14.0/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.14.0`, then commit what it changes and merge it, as an ordinary PR.
3. Run `pm check`. For each link it names, point the link at an existing record's file, or at a doc's or a postmortem's
   slug, or remove the link. Then commit the fixed records with `pm commit`.
4. For each JSX line it names in raw HTML, apply the fix the error gives: `class=` for `className=`, `<!-- -->` for
   `{/* … */}`, the computed value in quotes for `attr={…}`, the SVG name such as `stroke-width` for `strokeWidth`, and
   no blank line between `<svg` and `</svg>`. Then commit the fixed records with `pm commit`.

### Breaking changes

- `pm check` and `pm commit` refuse a store in which a relative `.md` link in a record names no record. The error
  names each file and its link. The page still renders on the site. Before, such a link was a 404 on the site and
  nothing reported it.
- `pm check` and `pm commit` refuse a record whose raw HTML, outside code, holds JSX: `className=`, a `{/* … */}`
  comment, an `attr={…}` value, or a camelCase SVG attribute such as `strokeWidth`, `textAnchor` or `fontSize`; or a
  blank line between `<svg` and its `</svg>`, which ends the HTML block and turns the rest of the drawing into text.
  Each error names the file, the line and the HTML fix. Records are CommonMark: pm does not support MDX. Before, such
  HTML rendered broken and nothing reported it.

### Added

- Math: `$…$` inline and `$$…$$` display math reach the page exactly as written, and KaTeX typesets them in the
  browser. KaTeX loads from a CDN at one exact version, only on pages with math. A `$` in code, a price such as `$5`
  and an escaped `\$` stay text.
- A docs index at `docs/index.html` lists every doc, newest first, with its date, its project and its bead. Every
  page's navigation links to it.

### Fixed

- An inline SVG with no width of its own fills the column, up to 960px wide.
- A relative link to a doc or a postmortem by its slug alone, without the date (`[x](note-foo.md)`), opens that
  record's page and is no longer a 404.

## [0.13.0] - 2026-10-15

Each worktree edits records in its own records workspace, so no session's commit carries another's edit, and the site
shows each worktree's edits as a version of its own.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.13.0/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.13.0`, then commit what it changes and merge it, as an ordinary PR.
3. Before moving the pin, commit or revert the record edits left uncommitted in the store
   (`git -C .pm/store/records status`): once a worktree's session starts on the new pin, its `records/` is its own
   workspace, and a design page left in the store can no longer be committed there.

### Breaking changes

- `pm commit` in the main checkout refuses a design page: design pages are edited and committed in a worktree's
  records workspace.

### Added

- Every worktree but the main checkout gets a records workspace at `<worktree>/records`, a worktree of the branch
  `drafts/<worktree>`, made by `pm init` (and so at session start) in place of the link to the store; the main
  checkout keeps the link. `pm where` states the workspace: its branch, uncommitted files, conflict markers, and how
  far it is behind the records branch.
- `pm commit` from a worktree first brings its workspace up to date with the records branch, merging the branch's
  changes into your uncommitted edits by `git merge-file`; where both changed the same lines it leaves conflict
  markers and refuses, naming the records commits that changed the file under your edit. It then commits the named
  files on the records branch in one commit, and refuses a file the shared store has uncommitted edits to. Every
  other pm write from a worktree commits on the records branch, then brings the workspace up to date the same way.
- A design page committed from a worktree is tagged with its sprint, the open sprint that links it in its Design
  pages and in which the session holds a task (`pm commit --sprint ID` names one of several), in the commit's
  message.
- The site shows each workspace's committed and uncommitted edits as a version of each page they change,
  labelled with its worktree, branch, the sprints its live session holds tasks in, and the session, and its state:
  committed, uncommitted edits or conflict.

### Changed

- The Stop hook in a worktree blocks once while its records workspace has uncommitted edits, naming each file and
  any conflict markers; in the main checkout it still checks the store's files this session's tool calls name.
- Records workspaces need git 2.40 or newer (`git merge-tree --merge-base`). With an older git, `pm init` leaves a
  worktree's `records/` the link to the shared store and warns, naming the version found and the one needed, at session
  start and in `pm doctor`; the rest of `pm init` runs.
- A figure the records branch changed under a worktree's uncommitted copy cannot merge: `pm commit` refuses it once,
  naming those commits, and a second `pm commit` replaces the published figure.

- `pm clean` removes a worktree's records workspace with it, and keeps a worktree whose workspace has uncommitted
  edits. `pm uninstall` removes the workspaces and their branches, refusing while one has uncommitted edits.
- Claude Code needs no `additionalDirectories` entry in a worktree any more: `pm init` takes the store out of a
  worktree's `.claude/settings.local.json` and lists it only in the main checkout's.

## [0.12.1] - 2026-10-15

The site shows each record as published on the `records` branch, and every draft of it beside it as a version.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.12.1/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.12.1`, then commit what it changes and merge it, as an ordinary PR.

### Added

- The store's uncommitted record edits are a draft, `store`. A page that a draft changes carries a version bar; the
  draft's version renders in full at `/drafts/store/<record>.html`, its links staying in the version, and its diff
  against the published text, section by section, at `/drafts/store/<record>.diff.html` (`?vs=<draft>` diffs two
  drafts). A draft that does not render fails only its own version's pages; its diff still shows.

### Changed

- The site renders the records committed on the `records` branch, not the store's working tree: a hand edit no
  longer shows as published before `pm commit`, and a record that does not parse no longer turns every page into an
  error.

## [0.12.0] - 2026-10-15

`pm interactive` turns the owner-request check off for one session while the owner chats in it, and `pm autopilot`
turns it back on; the request rule now follows the session's mode.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.12.0/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.12.0`, then commit what it changes and merge it, as an ordinary PR.
3. In Codex, trust pm's two new hooks in `/hooks`: `pm prime --request-rule` on SessionStart and `pm hook session-end`
   on SessionEnd, which `pm upgrade` adds after pm's existing entries in `.codex/hooks.json`. Codex skips them until
   you trust them; pm's other hooks keep their trust, and the session-start state names the two while they are
   skipped.

### Added

- `pm interactive` and `pm autopilot` set the mode of the session that runs them, in Claude Code and Codex. In
  interactive mode the owner-request Stop hook lets asks in chat through for that session alone, without running its
  judge, until `pm autopilot`. Sessions start in autopilot. An agent runs them when the owner asks for the mode;
  nothing enforces that. The uncommitted-records Stop check stays on in both modes. The mode lives in
  `<main checkout>/.pm/run/sessions/`, local to the clone.
- `pm hook session-end`, on SessionEnd in both runtimes, removes an ended session's mode. `pm clean` removes a mode
  that a crashed session left once the session has been idle for a day.

### Changed

- The rule for asking the owner moves out of `pm prime`'s rules: a SessionStart hook of its own,
  `pm prime --request-rule`, gives the rule of the session's mode (never leave a request only in chat, in autopilot;
  ask in chat when you need to, in interactive mode) at startup, resume, clear and compact, and `pm interactive` and
  `pm autopilot` print the rule of the mode they set.

## [0.11.1] - 2026-10-15

A design page or a doc can be renamed in one command that keeps every record's links to it working.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.11.1/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.11.1`, then commit what it changes and merge it, as an ordinary PR.

### Added

- `pm design rename <slug> [--to NEW_SLUG] [--title "…"]` moves `records/design/<slug>.md` to its new slug, sets its
  title, or both, and in the same records commit rewrites every record's link to the page (anchors kept, a link whose
  text was the old title takes the new one) and every `pm design unchanged` waiver of it. It refuses while an open PR
  review need names the page with `--design`.
- `pm doc rename <doc> [--to NEW_SLUG] [--title "…"]` does the same for a doc, named by its path, `<date>-<slug>` or
  slug; its date prefix stays. The feedback doc is not renamed.

## [0.11.0] - 2026-10-14

An owner's reply reaches the session that asked by the pm service's push alone, with `pm show`'s flag and `pm reply
read` for a session it did not reach; `pm reply wait` and the UserPromptSubmit hook `pm hook inbox` are gone.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.11.0/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.11.0`, then commit what it changes and merge it, as an ordinary PR.

### Removed

- `pm reply wait <id>`. In the transcripts on the owner's machine it never delivered a reply that the push had not
  delivered already. A raising command now names the push, or, for a session with no inbox, the `pm show` flag and
  `pm reply read <id>`.
- `pm hook inbox`, the UserPromptSubmit hook in Claude Code and Codex. `pm upgrade` to this version takes it out of
  `.claude/settings.json` and `.codex/hooks.json`, and so does `pm init` in a repo without `.pm/config.toml`; `pm init`
  in an installed repo, session start's included, leaves those files alone. Your own UserPromptSubmit hooks stay.
  `pm doctor` reports a file that still holds it.

## [0.10.0] - 2026-10-14

A question an agent asks with its runtime's ask tool shows on the owner's site too, where the owner can answer it as
well as in the session's chat; the first answer reaches the session. pm installs no git hook and no longer sets
`core.hooksPath`: session start's `pm init --session-start` sets each new worktree up, as it already did.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.10.0/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.10.0`, then commit what it changes and merge it, as an ordinary PR.
3. Move every clone of the repo to this release together. The work store's schema moves to version 4 the first time
   this release opens it, and the store syncs through the remote, so a clone still on an older pm fails its sync with
   "the remote's schema is version 4, newer than this pm's 3: run pm upgrade" until its main checkout has the new pin.
4. Where Codex works in the repo, open Codex in the main checkout and trust pm's hooks again in `/hooks`: `pm upgrade`
   adds a PostToolUse entry and a UserPromptSubmit entry to `.codex/hooks.json`, and Codex skips a new hook until it
   is trusted. `pm doctor` names any of pm's hooks Codex would skip.

### Added

- A question an agent asks with AskUserQuestion (Claude Code) or `request_user_input_async` (Codex) is raised as a
  new kind of need, a question, under the newest open task the session holds, or under none when it holds none. The
  site shows it in a "Sessions ask you" section above the decisions, with each question's options to pick and an
  "Other" field for the owner's own answer; `pm show` lists it too.
- The owner answers a question on the site or in the session's chat, and the first answer wins. In Claude Code a
  site answer closes the open question dialog with that answer; in Codex the pm service queues it into the session
  with `codex queue`, unless the owner already answered in the chat. A question answered in the chat closes on the
  site at once; Esc on Claude Code's dialog dismisses it.
- `pm hook ask --runtime claude|codex`, which raises and closes the questions. `pm upgrade` writes it into
  `.claude/settings.json` as a PermissionRequest hook on AskUserQuestion (timeout one day, so the owner can answer
  from the site for that long) and a PostToolUse hook on AskUserQuestion, and into `.codex/hooks.json` as a
  PostToolUse hook on `request_user_input_async` and a UserPromptSubmit hook.
- A question's answer steers the session that asked and records no decision: `pm decision add --need`,
  `pm decision close`, `pm action done` and `pm need edit` refuse a question and name `pm need dismiss`, which closes a
  stale one.

### Changed

- pm's rules choose how an agent asks the owner by what it asks, not by whether the owner is in the chat: a question
  that steers the session (what to do next, confirm a plan, clarify what the owner meant) goes through the runtime's
  ask tool, or as a decision need where there is none (`claude -p`, a subagent); a decision on a project's or sprint's
  outcome is a decision need; a step only the owner can do is an action need. The owner-request hook's block reason
  says the same.

### Removed

- pm's git post-checkout hook, which linked a new worktree's `records/` on `git worktree add`, is retired with the
  clone's `core.hooksPath` setting that ran it. `pm upgrade` removes pm's section from `.pm/hooks/post-checkout` (the
  file goes when nothing else is left in it, and `.pm/hooks` when it is empty); `pm init`, at every session start too,
  `pm upgrade` and `pm uninstall` unset `core.hooksPath` when it names `.pm/hooks` and nothing but pm's sections is
  there. A hooks path naming another directory, or a `.pm/hooks` holding hooks of your own, is left alone, and
  `pm init` no longer refuses one. Until every checkout of a clone pins this release, a session on an older pin sets
  `core.hooksPath` back to `.pm/hooks`, and one on this release unsets it again. `pm doctor` names a leftover file or hooks path. A worktree
  made without an agent session gets its `records/` link from `pm init` there. `pm where` drops its hooks line, and
  `pm hook git-post-checkout` does nothing, for the hook files older pins still run.

## [0.9.1] - 2026-10-14

Agents open the project, sprint or task for work the owner asked for without asking leave, as they already commit,
push and open PRs.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.9.1/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.9.1`, then commit what it changes and merge it, as an ordinary PR.

### Changed

- `pm prime`'s rules add the invariant that agents open the project, sprint or task for work the owner asked for
  without asking: the request is the leave, and a request that states the goal confirms it. When the goal is unclear,
  asking what it is is still a fair question; asking leave to open is not.
- The owner-request Stop hook blocks a reply that asks leave to open a project, sprint or task for work the owner
  asked for ("Shall I open a sprint for this?") as a needless ask, and tells the agent to open it. Asking whether to
  open one for work the owner did not ask for, or asking what the goal is, still has to be raised as a need.

### Fixed

- The owner-request judge on Codex matches a reply's request to the open need that asks the same thing when the reply
  writes another id for it, and passes a PR review the reply cites by id as open on the site, which it blocked now
  and then.

## [0.9.0] - 2026-10-14

pm reads and writes JSON with Go's own encoder now that Python pm is retired, and quotes a value in its messages the
Go way; the files pm writes in a repo stay byte for byte the same when their text is ASCII. pm reads each option only
by its whole name.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.9.0/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.9.0`, then commit what it changes and merge it, as an ordinary PR.

### Changed

- A message that quotes a value quotes it in double quotes, Go's way, instead of Python's single quotes: for example
  `no open project "old"; open ones: demo` (was `'old'`), `--merged "HEAD" is not a commit sha`, and the records
  commit `pm: renamed sprint <id> to "Sprint 1: …"`. The wording around the value is unchanged. A record header's list
  value shows its strings the same way (`covers is a list of path patterns …; got "internal/**"`), and a hook input of
  the wrong type is named by its JSON type (`last_assistant_message is a number`, was `a int`).
- `pm show --json` writes non-ASCII text as it is (UTF-8) instead of `\uXXXX` escapes; its layout is unchanged.
- The hooks' JSON output (`pm prime --hook-json`, `pm hook inbox`, `pm hook stop`, `pm hook owner-request`) is compact
  (`{"decision":"block","reason":"…"}`) and keeps non-ASCII text as it is. Claude Code and Codex read it as before.
- The site's `/version` answer is compact JSON, and its time drops a trailing `.0`.
- `.pm/config.toml` keeps a non-ASCII value (a remote or branch name, say) as it is instead of a `\uXXXX` escape; a
  character outside the Basic Multilingual Plane was written as a pair of escapes TOML does not read, and is now
  written as it is. With a non-ASCII value, `pm doctor` names the file as differing until `pm upgrade` rewrites it.
- `.claude/settings.json`, `.codex/hooks.json` and `.claude/settings.local.json` keep a number as the file writes it
  (`1.50` stays `1.50`) where pm rewrote it the way Python writes numbers, and `pm init` no longer refuses a file
  only for such a number. A string holding U+2028 or U+2029 is now written with that character escaped, so `pm init`
  refuses a file that holds either one raw as not laid out as pm writes JSON.

### Removed

- An abbreviated option is refused: `pm task add --tit x` no longer means `--title x` but fails with
  `unrecognized arguments: --tit x`, as any unknown argument does. Write each option in full. A prefix could change
  meaning when a command gained an option that shares it, and no agent or doc in use relied on one. A command line
  with both an unknown argument and a missing required one now names the unknown one.

## [0.8.8] - 2026-10-13

`pm check` and its `--help` state the sprint-close rule for covers patterns as pm 0.8.6 applies it.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.8.8/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.8.8`, then commit what it changes and merge it, as an ordinary PR.

### Changed

- `pm check`'s line for covers patterns waiting on unmerged work, and `pm check --help`, now say that `pm sprint close`
  holds a sprint only to the patterns it added or changed while open, not to every pattern on its pages.

## [0.8.7] - 2026-10-13

The site no longer serves a page set missing a record while a records sync, or another git command, is rewriting the
records store.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.8.7/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.8.7`, then commit what it changes and merge it, as an ordinary PR.

### Fixed

- The pm service's lock-free read of the records repeats under the records lock when git held the store worktree's
  index lock during it, as a checkout or a rebase does while it rewrites the files. A git that stalled between
  unlinking a record and writing it again left that file missing from the read and from both of its checks, so the
  read passed without it.

## [0.8.6] - 2026-10-13

`pm sprint close` holds a sprint only to the design-page `covers:` patterns it added or changed, so another session's
pending pattern no longer blocks an unrelated sprint's close. The pm service no longer dies with "panic: close of
closed channel" when a garbage collection follows another one closely.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.8.6/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.8.6`, then commit what it changes and merge it, as an ordinary PR.

### Fixed

- `pm sprint close` refused any sprint that held a design page with a pending `covers:` pattern, even one another
  sprint added for a file its open PR adds. It now checks against the remote's main only the patterns the page did
  not hold at the last records commit before the sprint opened: those the sprint added or changed, and every pattern
  of a page made during the sprint.
- A garbage collection of the work store now returns only once its visits to the other open sessions have ended.
  Dolt's ended a visit after the collection returned, so a collection that began right after another could visit a
  session whose last visit had not ended, and the pm service died ("panic: close of closed channel", inside Dolt).
  The service collects once a day, so this hit back-to-back collections, as pm's own tests run them.

## [0.8.5] - 2026-10-13

`pm check` no longer fails for every session while a design page names a file that only an unmerged branch has; `pm
sprint close` holds the page to the remote's main instead, once the sprint's PR is merged.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.8.5/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.8.5`, then commit what it changes and merge it, as an ordinary PR.

### Changed

- `pm check` lists a design page's `covers:` pattern that matches no tracked file as waiting on unmerged work, and
  passes. A sprint brings its page to the new state before its PR merges, so a page names a file before main has it.
  A malformed pattern, and a `design_sources` pattern that matches no tracked file, still fail.
- `pm sprint close`, with and without `--merged`, fetches the remote's main and refuses while a design page in the
  sprint's Design pages, or one the design-page gate names for its PRs, has a `covers:` pattern that matches no file
  there. The refusal names the page and the pattern: fix the pattern, or merge the PR that adds the file first.

## [0.8.4] - 2026-10-13

A pin move that crosses several releases now shows the upgrade guide of each one before it writes anything, and moving
pins no longer leaves every old pm binary on the machine.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.8.4/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.8.4`, then commit what it changes and merge it, as an ordinary PR.

### Added

- `pm upgrade --to X` first prints the upgrade guide of every release the move crosses, after the repo's pin up to
  X, oldest first and grouped by release, before it writes anything. pm X carries every earlier release's notes, so
  this needs no network; when a crossed release's guide is missing from them, it fails and writes nothing.
- `pm upgrade` prunes the launcher's pins (`~/.local/share/pm/pins/<X>/pm`, about 113 MB each) that no checkout on
  the machine pins: no worktree of this clone, nor of any clone with a pm service unit. A pruned pin keeps its
  `sha256`, so a later download of it is still checked against the first one. When any of those checkouts cannot
  be read, it prunes none and says why. `pm doctor` names the pins that `pm upgrade` would prune, without failing.

## [0.8.3] - 2026-10-13

`pm upgrade` and `pm doctor` keep `design_sources` in `.pm/config.toml`, so a repo can set it.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.8.3/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.8.3`, then commit what it changes and merge it, as an ordinary PR.

### Fixed

- `pm upgrade` keeps `design_sources` in `.pm/config.toml`, and `pm doctor` no longer reports a config that sets it as
  drift. Before, pm's config writer dropped the key, so the next `pm upgrade` deleted it.

## [0.8.2] - 2026-10-13

A PR review closed before its PR reached main, as a stacked PR's is, can be restamped once the PR's change is on main,
so its sprint can close; and `pm action need --pr` refuses a stacked PR, whose review could never close.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.8.2/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.8.2`, then commit what it changes and merge it, as an ordinary PR.

### Added

- `pm action done <review> --reason "merged as <sha>"` restamps a PR review closed without `merged as <sha>`, as when
  its PR merged into a stacked base: once `<sha>` is on the remote's main branch (pm fetches it first, as
  `pm sprint close --merged` does), the review's close reason becomes the new one with the old one kept after it, and
  its sprint can close. Any other closed action is refused as before.

### Changed

- `pm action need --pr` refuses a GitHub PR whose base branch is not the main branch: a stacked PR merges into another
  PR's branch, so its review could never close as merged on main. The refusal names the fix: raise the review on the
  PR that reaches main, or retarget this one (`gh pr edit <url> --base main`). pm asks `gh pr view` for the base, so
  raising a review on a GitHub PR now needs `gh`.

## [0.8.1] - 2026-10-13

A design page can name the code it covers, and a PR that changes that code waits until its sprint updated the page or
recorded why it stays as it is. pm releases on every merge to main that carries a changelog entry, with no one cutting
the release, and the version says whether moving a pin needs action: a minor release asks for a step beyond installing
it and `pm upgrade`, a patch release asks for none.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.8.1/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.8.1`, then commit what it changes and merge it, as an ordinary PR.

### Added

- A design page can name the code it is about in its header: `covers: [internal/records/**, prime.md]`, path
  patterns relative to the repo root (`*` within one path segment, `**` as a whole segment for any depth). `pm check`
  fails on a pattern that matches no tracked file.
- `pm action need --pr` and `pm sprint close` refuse a PR that changes a file a design page covers until one of the
  PR's sprints either updated the page (committed it since the sprint opened and links it in its Design pages) or
  recorded why it stays as it is with the new `pm design unchanged <slug> --sprint ID --reason "…"`, a sprint
  decision. They ask GitHub (`gh api`) for the PR's files only when some page has `covers:`, a renamed file's old path included. The review's text links each page
  covering the changed code.
- `design_sources` in `.pm/config.toml`, a list of path patterns: `pm check` lists the tracked source files it matches
  that no design page covers.
- A design page on the site says when the code it covers changed on the remote's main branch (as the clone last
  fetched it) after its last edit: how many commits, and the latest one's date. The merge of a sprint that updated
  or waived the page does not count. A clone that never fetched the remote's main shows "Code changes not checked"
  instead.

### Changed

- A release is published by CI on every merge to main that leaves an entry in `changelog.d/`, once every check on
  the merge commit passed. A merge with no entry releases nothing, and a `release/hold` file on main, naming the
  feature it waits for, holds every release until the PR that completes the feature deletes it.
- While pm is 0.x, a release is a minor one only when it asks a repo that moves its pin for a step beyond installing
  the release and `pm upgrade --to X` (its upgrade guide is longer than those two steps) or removes something; every
  other release, new features included, is a patch. 1.0 and later majors are cut by hand.
- Each repo still moves its pin when it chooses, by its own PR; `pm doctor` names a newer release.

## [0.8.0] - 2026-10-13

pm works in OpenAI Codex as it does in Claude Code: session identity, hooks, the owner-request judge on Codex's own
model, owner replies that wake an idle Codex session, and one project worked by both runtimes at once.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.8.0/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.8.0`. Besides the pin, it rewrites `.claude/settings.json` and
   `.codex/hooks.json`: `pm hook owner-request --runtime claude` and `--runtime codex` (with a 45 s timeout), a new
   `pm hook inbox` on UserPromptSubmit, and in Codex SessionStart on `compact` too and a SubagentStop hook. It also
   writes `.codex/rules/pm.rules`. Commit all of it with the pin move and merge it, as an ordinary PR.
3. Where Codex works in the repo, run `pm service install` from a shell where codex 0.149.0 or later is on PATH, so
   owner replies reach Codex sessions. `pm doctor` names a missing or older codex.
4. Where Codex works in the repo, open Codex in the main checkout, trust the project if it is not yet trusted, then
   run `/hooks` and trust pm's hooks, the new and the changed ones. Until then Codex skips them. `pm doctor` names any
   of pm's hooks Codex would skip.
5. In a repo whose machines have Codex and not Claude Code, set `summary_runtime = "codex"` in `.pm/config.toml` and
   commit it.

### Added

- `summary_runtime` in `.pm/config.toml`, `claude` (the default) or `codex`: the runtime `pm day summarize`, which the
  pm service runs every 10 minutes, asks. On `codex` it runs `codex exec` on `gpt-5.6-luna`. It never falls back to
  the other runtime; any other value fails the config check.
- `pm doctor` fails when the CLI `summary_runtime` names is not on PATH, since the pm service summarizes with it. It
  checks no judge's CLI: each judge runs only in its own runtime's sessions.
- A pm command in a Codex session whose rollout holds no pm session-start context, because Codex skipped pm's hooks
  (an untrusted hook is skipped silently), prints one warning on stderr, once per session, naming the fix: trust
  pm's hooks in `/hooks` and start a new session.
- The request rule names each runtime's ask tool: AskUserQuestion in Claude Code; `request_user_input_async` in Codex
  (Default mode), `request_user_input` in its Plan mode. The owner-request hook reads the stopping turn's ask tool
  calls from the session's transcript and gives their questions to the judge as asked, so a question put with the
  tool, or its repeat in the final reply (Codex's async question can surface there), is not taken for an ask left
  only in chat; any other chat ask in the same reply still blocks. A block reason names the session's own ask tool.
- A Codex session's start context opens with one line naming the pm hooks Codex skips (untrusted, changed since
  trusted, or disabled) and the fix, as after a `pm upgrade` rewrote an entry the user had trusted. pm asks Codex
  (`codex app-server`, `hooks/list`) only when `.codex/hooks.json` or the trust in the user's Codex config changed
  since the last answer, which `.pm/run/codex-trust.json` keeps.
- The pm service delivers an owner reply into a live Codex session as it pushes one into a Claude Code session: with
  `codex queue` under the CODEX_HOME the session raised the need with, which starts a turn when the session is idle
  (measured: 6.1 s after the reply in an idle Codex TUI). A Codex session's need records that CODEX_HOME; a session
  that has ended keeps the reply undelivered, and a codex missing from the service's PATH or older than 0.149.0 is
  named on the reply's card and by `pm doctor`.
- `pm reply wait <id> [--timeout DURATION]`: wait within the turn for a new reply from the owner to a need, then
  print it (and mark it delivered, as `pm reply read` does); a reply delivered before the wait is no new one, so an
  agent that asks again waits for the next. The output of a raised need names it.
- `pm hook inbox`, on UserPromptSubmit in Claude Code and Codex: hands a session the owner's replies that did not
  reach it as context beside its next prompt, and marks them delivered once printed.
- `pm init` writes `.codex/rules/pm.rules`, which runs `pm` outside Codex's sandbox: a pm command reaches the pm
  service through a socket that a network-off sandbox blocks. A command Codex cannot split into plain commands, such
  as one fed a heredoc, still runs inside it.
- `pm doctor` asks Codex (`codex app-server`) which of pm's hooks it would skip, untrusted, changed since trusted or
  disabled, or whether it runs none of the project's, and names them with the fix; a typed `pm init` names them too.
  pm never writes Codex's trust. Without `codex` on PATH, or without `$CODEX_HOME` (Codex never ran on the machine,
  and the call would make its state), it says the trust was not checked.

### Changed

- The owner-request Stop hook's judge runs on the session's own runtime, named by each hook entry
  (`pm hook owner-request --runtime claude|codex`): Claude Haiku through `claude -p` in Claude Code, `gpt-5.6-luna`
  through `codex exec` in Codex. A Codex session no longer needs `claude`; without it the judge failed, and Codex
  took the failure as a silent pass. The Codex judge runs from an empty temp dir with Codex's hooks, rules and your
  `config.toml` off and saves no session, so it cannot run the hook again. An entry without `--runtime` runs the
  Claude judge, as before.
- A timed-out judge or summary call now kills its whole process group, not only the `claude` or `codex` launcher, and
  a hook stopped by its runtime (SIGTERM, SIGINT or SIGHUP, as at the entry's timeout) kills its judge too.
- The day summary's prompt asks for at least one bullet, so a quiet day gets one on either runtime.
- A Codex subagent now shares its root thread's session, as a Claude Code subagent shares its session's: pm takes
  Codex's `$CODEX_SESSION_ID`, the root session's id, and with an earlier Codex that sets only `$CODEX_THREAD_ID`,
  maps the thread to its root through its rollout under `$CODEX_HOME`. So a subagent may claim, release and answer
  what its session holds and raised.
- With both Claude Code's and Codex's session set (Codex started from a Claude Code session inherits
  `$CLAUDE_CODE_SESSION_ID`), the session is that of the nearest Claude Code or Codex process pm runs under; it was
  always Claude Code's. With neither among pm's ancestors, pm refuses, naming both ids and which to unset.
- `pm task claim`'s refusal in the main checkout says its git commands make a worktree in any runtime.

### Fixed

- A Codex run started from a Claude Code session no longer stores that session's inbox
  (`$CLAUDE_CODE_MESSAGING_SOCKET`, which Codex inherits) on its needs, so its replies are no longer pushed into the
  Claude Code session; its needs carry its own CODEX_HOME.
- The pm service pushes a reply `pm reply add` stored as soon as the work store moves; it waited up to a minute for
  its next sweep.
- A need closed by `pm decision add --need` or `pm decision close` records the closing session, as other closes do.
- A Codex session's needs trade the Claude Code inbox an earlier pm stored on them for the session's CODEX_HOME, at
  its next session start.
- The site's reply to a need of a live session with no inbox says the reply waits for that session, at its next
  prompt or through `pm reply wait`; it said the session was not running. "Not running" is kept for a session whose
  transcript has ended.
- A typed `pm init` keeps an installed service unit's PATH: run from Codex, which puts its own directory first on
  every command's PATH, it rewrote the unit and restarted the clone's pm service under every live session (their pm
  commands failed meanwhile), and a Claude Code session's `pm init` flipped it back. `pm service install` takes the
  PATH it runs with.

- A compacted Codex session gets pm's rules and state again: Codex's SessionStart hooks ran on startup, resume and
  clear only.
- Records a Codex subagent left uncommitted block it at its stop (Codex's SubagentStop), as a Claude Code subagent's
  block its session's stop; Codex keeps a subagent's prompt from its parent's transcript, so the parent's stop could
  not tell.
- `pm hook owner-request` lets a Codex turn that ended without a message through; it failed the check on Codex's
  null `last_assistant_message`.
- `pm clean` keeps a worktree a live Codex session works in or names in a tool call; it read only Claude Code's
  transcripts.
- `pm reply read` without ids finds a Codex session's requests; it read only `$CLAUDE_CODE_SESSION_ID`.
- `pm doctor` reports pm's hook entries whose matcher an earlier pm wrote; it compared their commands only.
- Finding a Codex thread's transcript reads only the date directories the thread can have started on, not every
  rollout under `$CODEX_HOME/sessions`.

## [0.7.1] - 2026-10-11

The Cloudflare Access check accepts only an application token; fixes found by the new lint gates.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.7.1/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.7.1`, then commit what it changes and merge it, as an ordinary PR.

### Fixed

- A pm service that finds the work store held by another process names Dolt's own error as the cause, which it
  used to drop.
- `pm uninstall` fails on a service unit it cannot read, instead of reporting no service installed.
- A reply spool line that is JSON but not a spool entry is refused with its line number, instead of read as empty.
- `pm push` reports git's counts as unreadable when they are not numbers, instead of reading them as 0.

### Security

- The Cloudflare Access check accepts only an application token (`type` `app`). The login redirect hands anyone a
  `meta` token signed by the team's key for the site's `aud`, which 0.7.0 refused only because it carries no `iss`;
  it is now refused by rule, as is the team's global session token (`org`).

## [0.7.0] - 2026-10-11

The pm site's public host requires a valid Cloudflare Access token; a pin move names the worktrees it strands; a
stopping service waits for its loops; the day summary tells only what happened.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.7.0/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.7.0`, then commit what it changes and merge it, as an ordinary PR.
3. In a repo whose `.pm/config.toml` sets `site_url`, put the site behind a Cloudflare Access application and, in
   step 2's PR, add `access_team = "<team>.cloudflareaccess.com"` (the Access team domain) and
   `access_aud = "<AUD tag>"` (the application's AUD tag) to `.pm/config.toml` (0.6.0 refuses both keys, so they
   cannot land before the pin moves); once it merges, run `pm service restart` in each clone. Until then the site
   refuses every request through the tunnel; localhost is served as before. A repo without `site_url` needs no step.

### Breaking changes

- With `site_url` set, the pm service serves a request to its host, or any request Cloudflare's edge forwarded
  (it carries `Cf-Connecting-IP`, so a tunnel that rewrites `Host` to localhost is no way around it), only when its
  `Cf-Access-Jwt-Assertion` verifies: RS256 by a key from `https://<access_team>/cdn-cgi/access/certs`, `aud` holding
  `access_aud`, `iss` `https://<access_team>`, unexpired. It fails closed: without either key, or when the team's keys
  cannot be fetched, every such request gets a 403 saying why. The tunnel exposed the site, and a reply posted there
  becomes an instruction in an agent session; before this, anyone who reached the tunnel could read every page and post
  one (see step 3).

### Added

- `.pm/config.toml` takes `access_team` and `access_aud`, and `pm doctor` names the one a set `site_url` lacks.
- `pm upgrade` lists the clone's other worktrees whose checked-out branch pins another pm than the one it moves to,
  each with "merge or rebase it onto the pin move": once the main checkout pins the new version and the pm service
  runs it, pm refuses every work-store command in them.

### Changed

- In a checkout whose pin is not the version the clone's pm service runs, the refusal (and `pm where`'s work line,
  `pm doctor`'s work store line and `pm init`'s refusal) names which side of the pin move the branch is on and the fix:
  a branch from before the move merges or rebases onto it (`git rebase <remote>/<main branch>`); the branch that moves
  the pin waits for its merge. `pm where`'s service line says the service runs the main checkout's pin instead of
  calling it stale and naming `pm service restart`, and `pm doctor` no longer reports such a service as stale.
- The Today summary on a day page tells only what happened that day: it no longer ends with a "For you" bullet that
  tells the owner what to do. The open requests stay in the page's "Actions await you" and "Decisions await you"
  sections.

### Fixed

- A pm service that stops (its pin moved) waits for its own loops before it closes the work store: a reply or merge
  being written, the sweep, the merge watch, a sync or a gc under way. It used to close the store while one of them
  could still be writing to it.

### Security

- Every request now names this machine (127.0.0.1, localhost) or `site_url`'s host, or gets a 403: a page, and the
  reply token on it, is no longer served to a rebinding domain; only a reply was checked before. A 403 carries neither
  the store-path header nor the build header.

## [0.6.0] - 2026-10-10

Every command pm runs is now listed in `pm --help` and in the noun list `pm prime` injects, the work-store commands
(`pm task ready`, `pm dep`, `pm need dismiss`, `pm sync` and the rest) included. New commands rename a sprint
(`pm sprint edit`), fix a need before the owner replies (`pm need edit`) and close a sprint whose PR merged with no
review (`pm sprint close --merged`); `pm finding add` takes `--text`, `pm commit` takes paths relative to the store,
`pm check` refuses a `.md` link to nothing, two intermittent failures are fixed at their root (the site reading the
records while a sync rewrote them, and the work store's clone racing git's detached maintenance), and pm is built
with Go 1.26.9.

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.6.0/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.6.0`, then commit what it changes and merge it, as an ordinary PR. It changes
   only the pin in `.pm/config.toml` and the version in pm's section of `.pm/hooks/post-checkout`: the hook entries
   and every other piece pm manages are as 0.5.0 wrote them. Until step 3, pm in that branch's worktree refuses every
   work-store command, since the clone's service still runs 0.5.0 ("this checkout pins pm 0.6.0, but the clone's pm
   service runs pm 0.5.0 …"): run those from the main checkout.
3. In each clone, once the PR is on the main branch, pull it into the main checkout, then run `pm service restart`.
   Session start never restarts a running service, and until the restart every work-store command in the main
   checkout fails with "the pm service runs pm 0.5.0, not pm 0.6.0: run pm service restart".
4. In each clone, merge the main branch into every worktree's branch cut before the merge (or rebase it onto the pin
   move). Once the service runs 0.6.0, pm in a worktree whose branch still pins 0.5.0 refuses every work-store
   command, session start's included ("this checkout pins pm 0.5.0, but the clone's pm service runs pm 0.6.0 …").

### Breaking changes

- `pm finding add` with no finding text exits 1 with "the finding text is empty", as its other refusals do, where
  0.5.0 exited 2 with a usage error. A finding text that starts with `--`, which 0.5.0 stored as the finding
  (`pm finding add --sprint ID -- "--txt=…"`), is refused with exit 1: it is a misspelt option, not a finding.
- A `.md` link to nothing in the records store (a symlink whose file is gone) is an error, naming the link and its
  target, for `pm check`, `pm commit`, the site and every other read of the records as they are on disk; 0.5.0
  passed over it, and `pm commit` committed it as a record. Remove the link or restore its file.

### Added

- `pm finding add` takes the finding with `--text` or `--text-file` too, as every body command does, and refuses a
  text that starts with `--` (a misspelt option such as `--txt=…`), which it used to store as the finding.
- `pm sprint edit ID --title "…"` renames an open sprint: its work-store title, which keeps its `Sprint <n>: `, and
  its record's title, with the reason (the body, two lines or more) recorded as a sprint decision.
- `pm need edit ID` rewrites an open need's body before the owner replies: an action's description with `--text` or
  `--text-file`, or a decision need's parts with the flags `pm decision need` takes, checked the same way. It refuses
  a need that holds a reply, a closed need and a PR review.
- `pm sprint close ID --merged SHA [--pr URL]` closes a sprint whose PR was merged with no review need and stamps
  `Merged as <sha> (PR #N).` into its Outcome, as a close after a review does. The commit must be on the remote's
  main branch, which pm fetches first; a sprint that holds a review is refused.

### Changed

- pm is built with Go 1.26.9 (was 1.26.2).

### Fixed

- `pm --help`, each `pm <noun> --help` and the noun list `pm prime` injects name every command pm runs:
  `pm dep`, `pm need`, `pm comment`, `pm sync`, `pm export` and `pm version`, and `pm task ready`, `edit` and
  `release` and `pm reply add`, which ran but were listed nowhere (`pm dep --help` said "invalid choice"). `pm show
  --help` names `pm show ID`, `pm task add --help` names `--parent`, and `pm init --help` names `--import-bd` and
  `--import`. Their flags and behaviour are unchanged.
- `pm decision close` on a need the owner never answered, and its `--help`, name `pm need dismiss` for a need that
  became moot.
- `pm task claim --help` says what `--session` does: it is the session recorded, even when the environment names one.
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
- `pm commit` takes a path relative to the store, such as `sprints/x.md`, from anywhere, as it takes
  `records/sprints/x.md`.

## [0.5.0] - 2026-10-10

Records now live only on the `records` branch: the main branch's copy of `records/` is gone, with the workflows, the
`pre-commit` section and the sparse checkout that kept it. A repo keeps one pm feedback doc, the Stop hooks block
only what is the agent's to fix, and new commands remove agent worktrees nobody needs (`pm clean`), move a sprint to
another project (`pm sprint move`), stop the pm service (`pm service stop`) and close a task that will not be done
(`pm task close --dropped`).

### Upgrade guide

1. On each machine, install the release:
   `curl -fsSL https://github.com/Yeeef/pm/releases/download/pm-v0.5.0/install.sh | sh`.
2. In the repo, run `pm upgrade --to 0.5.0`. Besides moving the pin and rewriting pm's hook entries (a third rules
   hook), it deletes `.github/workflows/pm-records-copy.yml` and `.github/workflows/pm-records-guard.yml` and pm's
   section in `.pm/hooks/pre-commit` (the file goes when nothing else is in it), and the commit it prints also
   untracks the main branch's `records/` copy (`git rm -r -q --cached --sparse --ignore-unmatch records`; the records
   stay on the `records` branch, and each worktree's link stays). Make that commit and merge it as an ordinary PR.
   Until step 3, pm in that branch's worktree refuses every work-store command, since the clone's service still runs
   0.4.0 ("this checkout pins pm 0.5.0, but the clone's pm service runs pm 0.4.0 …"): run those from the main
   checkout.
3. In each clone, once the PR is on the main branch, pull it into the main checkout, then run `pm service restart`.
   Session start never restarts a running service, and until the restart every work-store command in the main
   checkout fails with "the pm service runs pm 0.4.0, not pm 0.5.0: run pm service restart".
4. In each clone, merge the main branch into every worktree's branch cut before the merge (or rebase it onto the pin
   move). Once the service runs 0.5.0, pm in a worktree whose branch still pins 0.4.0 refuses every work-store
   command, session start's included ("this checkout pins pm 0.4.0, but the clone's pm service runs pm 0.5.0 …"),
   and such a branch still tracks `records/`. After the merge, `pm init` (session start runs it) turns off the sparse
   checkout an earlier pm set in that worktree.
5. Move every clone, and every worktree's branch, to the new pin (steps 3 and 4) before anyone runs
   `pm feedback add`, `pm sprint move` or step 6: an earlier pm fails every command that reads the records once
   `records/docs/pm-feedback.md` exists, since it reads that doc's header (no date in its name, no project) as
   invalid, and the records store syncs to every clone; and a clone on 0.4.0 fails its sync once it changed a sprint
   that another clone moved, since 0.4.0 holds a sprint's number fixed.
6. If the records store holds pm feedback docs from an earlier release (`records/docs/<date>-<project>-feedback.md`,
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
  gets that copy where its link goes: `pm init`, `pm doctor` and `pm where` say to merge the main branch (step 4).
- A repo keeps one pm feedback doc, `records/docs/pm-feedback.md`. Any other doc titled `pm feedback`, such as the
  per-project docs earlier releases wrote, fails `pm check` and `pm commit`, and `pm feedback add` refuses, until it
  is merged into the one doc (step 6). Feedback about pm is read in one place, whatever project it is about.
- An earlier pm cannot read a records store that holds `records/docs/pm-feedback.md`, and a clone on 0.4.0 fails
  its sync over a sprint it changed that `pm sprint move` moved: move every clone to this release first (step 5).

### Added

- `pm clean` lists every worktree of the clone with keep or remove and the reason, and `pm clean --apply` removes
  the agent worktrees under `.claude/worktrees/` that nobody needs: clean, with their branch on `origin/main`
  (squash merges checked with `gh`) or pushed with nothing beyond. It keeps a worktree that a live process locks
  (Claude Code's lock, with its pid and start time) or that a live session used in the last 30 minutes, one with
  uncommitted changes or commits never pushed, the main checkout, the records store and the worktree it runs in.
  It removes with `git worktree remove` (never `--force`) and deletes a branch only once it is merged.
- `pm task close <id> --dropped --reason "…"` closes a task that will not be done: no commit, no warning, and
  `pm show --sprint`, the site's task graph and day pages show it as dropped (it is stored with the `dismissed`
  resolution every clone already accepts, so no store migration). A task it blocks no longer waits on it.
- `pm task close --commit` takes `OWNER/REPO@SHA` for work committed in another repo, or a PR URL, both resolved with
  `gh`; one that does not resolve, or a PR closed without merging, is refused.
- The site serves image files (`.svg`, `.png`, `.jpg`, `.jpeg`, `.webp`) kept in the records store, next to the
  record's page, so a record shows a figure with `![…](fig.png)` instead of a base64 data URI. Paths that leave the
  store (`..`, a symlink out of it) or name a dot file get 404. `pm check` and `pm commit` take such files beside a
  record, and the stylesheet keeps images, figures and captions inside the column.
- `pm task move` takes a task filed directly under a project (the site's Not in a sprint) into one of that
  project's open sprints, recording the scope added as a decision in the sprint it joins.
- `pm service stop` stops a clone's pm service and keeps it stopped: it disables the unit at its supervisor
  (systemd or launchd), so neither login nor a crash starts it again, waits until the work store's socket and the site
  no longer answer, and says so. Session start leaves a stopped service stopped and its state names
  `pm service restart`; a typed `pm service restart` or `pm init` starts it again.
- `pm uninstall` works with the pm service stopped: it runs a `pm service run` of its own for its unsynced-work check,
  so it still refuses a work store that holds what the remote lacks.
- `pm record link --local` prints the record's `http://127.0.0.1:<port>/…` URL, which the clone's service answers on,
  even when `site_url` names a public site behind a login, for a check from this machine (a headless browser).
- `pm doctor` names a pm release newer than the repo's pin, with its notes' URL and the move (`pm upgrade --to X`),
  or says that the release list cannot be read. The line does not change doctor's exit code.
- `pm sprint move <sprint> --to <project>` moves an open sprint, with its tasks, needs, frame, decisions, findings
  and report, to another open project in one command, and records the move as a decision in both projects. The
  sprint keeps its id, so its tasks keep their holders and `pm show <id>` finds it in its new place; it takes the
  project's next sprint number, and its record is renamed to match. The old record path and site page still lead to
  it, and the old project never reuses the moved number. If the move stops part way, run the same command again to
  finish it.

### Changed

- Session and subagent start inject pm's rules as three hooks instead of two: the first chunk had outgrown the
  10,000 characters Claude Code passes inline. `pm upgrade` (step 2) rewrites the hook entries as it moves the pin.
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
- `pm task close` refuses a task that another live session holds, as `pm task claim` does, and names
  `pm task release`.
- A sprint's "n of m tasks done" on the overview, and `pm show`'s n/m done, leave dropped tasks out of both
  numbers.

### Fixed

- Mermaid diagrams keep their natural width and scroll sideways, so a wide flowchart stays legible on a phone instead
  of shrinking its text to a few pixels; they are drawn again when the colour scheme changes; and the pages load one
  exact Mermaid release (11.17.2), from a second CDN when the first fails, instead of whatever `mermaid@11` is today.
- `pm task close --commit` no longer warns about uncommitted changes in the working tree: the named commit is the
  work.
- `pm init --help` now says what `--session-start` does with the service: it starts an installed service that does
  not answer, as session start has done since 0.3.0, so a session in a clone brings its service up (unless
  `pm service stop` stopped it).
- The site no longer shows "error: open …: no such file or directory" in place of every page when a records sync
  rewrites record files while the service reads them: a record file, directory or day summary that is gone by the
  time it is read is left out, and the next look reads the files that replaced it.
- The pm service's garbage collection could delete the work store's live journal when a write or a sync raced it,
  losing the writes since and crashing the service at its next collection ("error dropping journal writer during
  UpdateGCGen"). pm now builds with a Dolt that holds the fix (dolthub/dolt#11312).
- The owner-request Stop hook no longer blocks text that asks the owner nothing: the agent's plan or progress
  (including what its own subagents do, and a review or merge it names itself as doing), a fact no step of the agent
  waits on, a request cited by its id as already open on the site (whichever session or clone holds it), and an
  answer to a question the owner asked. A PR review or merge left pending without saying who does it still counts as
  a request to the owner. When a reply asks again for what an open request already asks, the block now says to drop
  the ask or say by its id that the request is open, not to raise a duplicate.
- `pm hook stop` no longer blocks a session over a record that only the prompt of its still-running subagent names;
  the subagent may be writing it. Once the subagent returns, or a tool call of the session itself names the record,
  the hook blocks as before.

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

[0.15.0]: https://github.com/Yeeef/pm/compare/pm-v0.14.0...pm-v0.15.0
[0.14.0]: https://github.com/Yeeef/pm/compare/pm-v0.13.0...pm-v0.14.0
[0.13.0]: https://github.com/Yeeef/pm/compare/pm-v0.12.1...pm-v0.13.0
[0.12.1]: https://github.com/Yeeef/pm/compare/pm-v0.12.0...pm-v0.12.1
[0.12.0]: https://github.com/Yeeef/pm/compare/pm-v0.11.1...pm-v0.12.0
[0.11.1]: https://github.com/Yeeef/pm/compare/pm-v0.11.0...pm-v0.11.1
[0.11.0]: https://github.com/Yeeef/pm/compare/pm-v0.10.0...pm-v0.11.0
[0.10.0]: https://github.com/Yeeef/pm/compare/pm-v0.9.1...pm-v0.10.0
[0.9.1]: https://github.com/Yeeef/pm/compare/pm-v0.9.0...pm-v0.9.1
[0.9.0]: https://github.com/Yeeef/pm/compare/pm-v0.8.8...pm-v0.9.0
[0.8.8]: https://github.com/Yeeef/pm/compare/pm-v0.8.7...pm-v0.8.8
[0.8.7]: https://github.com/Yeeef/pm/compare/pm-v0.8.6...pm-v0.8.7
[0.8.6]: https://github.com/Yeeef/pm/compare/pm-v0.8.5...pm-v0.8.6
[0.8.5]: https://github.com/Yeeef/pm/compare/pm-v0.8.4...pm-v0.8.5
[0.8.4]: https://github.com/Yeeef/pm/compare/pm-v0.8.3...pm-v0.8.4
[0.8.3]: https://github.com/Yeeef/pm/compare/pm-v0.8.2...pm-v0.8.3
[0.8.2]: https://github.com/Yeeef/pm/compare/pm-v0.8.1...pm-v0.8.2
[0.8.1]: https://github.com/Yeeef/pm/compare/pm-v0.8.0...pm-v0.8.1
[0.8.0]: https://github.com/Yeeef/pm/compare/pm-v0.7.1...pm-v0.8.0
[0.7.1]: https://github.com/Yeeef/pm/compare/pm-v0.7.0...pm-v0.7.1
[0.7.0]: https://github.com/Yeeef/pm/compare/pm-v0.6.0...pm-v0.7.0
[0.6.0]: https://github.com/Yeeef/pm/compare/pm-v0.5.0...pm-v0.6.0
[0.5.0]: https://github.com/Yeeef/pm/compare/pm-v0.4.0...pm-v0.5.0
[0.4.0]: https://github.com/Yeeef/pm/compare/pm-v0.3.0...pm-v0.4.0
[0.3.0]: https://github.com/Yeeef/pm/compare/pm-v0.2.2...pm-v0.3.0
[0.2.2]: https://github.com/Yeeef/pm/compare/pm-v0.2.1...pm-v0.2.2
[0.2.1]: https://github.com/Yeeef/pm/compare/pm-v0.2.0...pm-v0.2.1
[0.2.0]: https://github.com/Yeeef/pm/compare/pm-v0.1.6...pm-v0.2.0
