A design-page draft made in one clone now shows on every other clone's site, and a sprint closed from any clone
publishes it.

### Added

- `pm init` gives each clone an id once (`git config pm.clone`), which `pm where` shows and `pm doctor` names when
  it is missing.
- The pm service's sync and `pm push` gain a `drafts` step: each records workspace's draft commit is pushed to the
  remote's `refs/pm/drafts/<clone id>/<worktree>`, forced only with a lease against what this clone last pushed
  there, and deleted once the draft is empty, published or gone; the other clones' are fetched. A lease that fails
  is flagged as a failed push in `pm show`, `pm where` and on the site, and never forced.
- The site lists another clone's draft as a committed version with no session, labelled with its clone, its
  worktree there and its sprint, at `/drafts/<clone id>~<worktree>/…`.
- `pm sprint close` fetches the other clones' drafts first and publishes their pages of the sprint too, under the
  same overlap and conflict refusals, naming the clone; the clone that drafted them drops them at its next follow and
  deletes its ref at its next sync. The design-page gate and `pm check` count another clone's draft as its sprint's.

### Changed

- `pm sprint close` refuses when it cannot fetch the other clones' drafts from the remote, and in a clone with a
  remote but no id (run `pm init`).
- The records step of the pm service's sync and `pm push` fast-forwards a records store that is only behind the
  remote's, where it used to leave it behind: never with a merge commit, and refused, flagged as a failed push, while
  an uncommitted edit in the store would be overwritten. Each records workspace with no uncommitted file then
  follows, so a draft drops the pages another clone's close published.
- A draft page whose sprint has closed with other text published (an edit committed after the clone last pushed)
  leaves the draft commit at the next follow and stays in the workspace as an uncommitted edit; `pm where`, the
  site's version bar and the Stop hook name it as stranded and say to commit it under an open sprint, or revert it.
