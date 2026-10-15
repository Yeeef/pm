Each worktree edits records in its own records workspace, so no session's commit carries another's edit, and the
site shows each worktree's edits as a version of its own.

### Upgrade guide

1. Before moving the pin, commit or revert the record edits left uncommitted in the store
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
