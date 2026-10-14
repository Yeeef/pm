---
type: sprint
title: pm retires its post-checkout git hook and stops owning core.hooksPath
bead: yeeef-agents-9va.132
---

## Goal

> What should be true when this sprint ends, and why now?

pm installs no git hook and no longer sets `core.hooksPath`: a new worktree is set up by the session-start hook (`pm init --session-start`), which already runs the same clone setup, or by `pm init` by hand. The owner chose this on 2026-10-14 in chat: the post-checkout hook only covered worktrees made with a checkout and used without an agent session, and it cost pm ownership of the clone's hooks path plus a hook file rewritten on every pin move.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `.pm/hooks/post-checkout` becomes a retired piece: `pm init`, `pm upgrade` and `pm uninstall` remove pm's section (and the file when nothing else is left), and pm unsets `core.hooksPath` when it points at `.pm/hooks`; `pm doctor` names a leftover.
- `SetupClone` no longer sets the hooks path; `pm where` drops its hooks line.
- `pm hook git-post-checkout` stays as a no-op while older pins' hook files still call it.
- Design pages that describe the hook (records-store, pm-product, pm-go, pm-cli, release-policy, record-drafts) edited to the new state; a changelog entry.

**Out:**
- Any other hook manager or a replacement git hook.
- Changes to the session-start hook.

## Done when

> What evidence will show the goal is met?

- `make test` and `make test-go` pass; the PR's CI is green.
- In a scratch clone set up by the previous release, `pm upgrade` to this build leaves no `.pm/hooks/post-checkout` and no `core.hooksPath`, and a new worktree gets its `records/` link from `pm init --session-start`.

## Design pages

> Where is the detail?

- [Shared records store](../design/records-store.md): Retired pieces (the post-checkout section and `core.hooksPath`), Setup and sync without a git hook.
- [pm as an installable product](../design/pm-product.md): Where pm's pieces live, Git hooks, Commands, and the git hook as an alternative not kept.
- [pm in Go](../design/pm-go.md): the package map, `pm init` and the repo's tracked pieces.
- [pm CLI](../design/pm-cli.md): `pm where` without a hooks line.
- [When and how pm releases](../design/release-policy.md): what a pin move rewrites.
- [Record drafts and publishing](../design/record-drafts.md): its setup facts without the git hook.
- [Work store](../design/work-store.md): the setup row and constraint row 8 (pm owns no hook directory).

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-14}
Retire the post-checkout git hook and pm ownership of core.hooksPath.
Session start already runs the same clone setup in every agent worktree; the hook only covered worktrees used without a session, at the cost of owning the hooks path and rewriting the hook on every pin move.
:::

::: decision {source=agent date=2026-10-14}
The design page pm-in-codex stays as it is for this sprint's code changes.
The PR changes only pieces.go's git-hook piece, which this page does not describe; its runtime hook entries are unchanged.
:::

::: decision {source=agent date=2026-10-14}
The design page session-claims stays as it is for this sprint's code changes.
The PR only drops the hook's name from the claim refusal's worktree hint; the claim rules this page holds are unchanged.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Live check of the retired post-checkout hook, in a scratch origin and clone
  under a temp dir with HOME, CODEX_HOME and systemctl faked as the test
  fixtures do (a scratch script driving each command): pm 0.8.6, the pinned
  release, run as `pm init` wrote `.pm/hooks/post-checkout` (section v0.8.6)
  and set `core.hooksPath=<clone>/.pm/hooks`. This build's `pm upgrade --to
  0.9.0-dev` printed "removed .pm/hooks/post-checkout, retired with pm's git
  hooks, as session start sets each new worktree up" and "unset
  core.hooksPath=<clone>/.pm/hooks, …"; after it `.pm/` held config.toml,
  .gitignore and README.md (no hooks/), `git config --get core.hooksPath`
  exited 1, and the status showed `M .pm/config.toml` and `D
  .pm/hooks/post-checkout`. A new `git worktree add` then had no `records/`
  (no hook ran); `pm init --session-start` there linked `records ->
  <clone>/.pm/store/records` (the 6 layout directories listed). `pm hook
  git-post-checkout <40 zeros> abc 1` exits 0 with no output. With both
  leftovers recreated by hand, this build's `pm doctor` named the two ("repo:
  .pm/hooks/post-checkout: an earlier pm's file, retired with pm's git hooks
  …" and "hooks path: core.hooksPath is <clone>/.pm/hooks, … run pm init to
  unset it"); `pm init --session-start` unset the path, `pm upgrade` removed
  the file, and `pm doctor` then named neither. The fake supervisor never
  brought the scratch pm service up, for pm 0.8.6 as for this build, so each
  `pm init` exited 1 at the service step after the clone setup had run; this
  change does not touch that step.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: pm installs no git hook and no longer owns `core.hooksPath`; session start's `pm init --session-start` sets each new worktree up, as it already did in every agent worktree.

- The post-checkout section is a retired piece: `pm upgrade` and `pm uninstall` remove it, and `.pm/hooks` once empty; `pm doctor` names a leftover.
- Every `pm init` unsets `core.hooksPath` when it names `.pm/hooks` and that directory holds only pm's sections; a directory with the user's own hooks keeps the path.
- `pm hook git-post-checkout` stays as a no-op for checkouts on older pins; `pm where` drops its hooks line.
- Known window: until every checkout pins this release, an older pin's session sets the clone-wide path back (noted in the changelog).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- `make test` and `make test-go` pass; the PR's CI is green: met. `make test` 135 passed, `make test-go` exit 0, `make test-full` on the init, lifecycle and pm tests 185 passed; PR #94's CI 13 of 13 checks pass on d1f6e35.
- Upgrade in a scratch clone leaves no hook and no `core.hooksPath`, and a new worktree gets `records/` from `pm init --session-start`: met. See the live-check finding: after this build's `pm upgrade`, `.pm/` had no `hooks/`, `git config --get core.hooksPath` exited 1, and `pm init --session-start` linked the new worktree's `records/`.
