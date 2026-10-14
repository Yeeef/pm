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

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
