---
type: sprint
title: Release pm 0.1.3
bead: yeeef-agents-9va.102
---

## Goal

> What should be true when this sprint ends, and why now?

pm 0.1.3 is released from main, so every session runs the overview Sprints list cap (PR #75).

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the version bump in .pm/config.toml, pm/pyproject.toml, pm/uv.lock and the two Beads hook markers; a PR with green CI.
**Out:** merge, tag, tool upgrade and service restart (owner steps after review).

## Done when

> What evidence will show the goal is met?

- The bump PR is on main with green CI, tag pm-v0.1.3 points at it, and `pm --version` in this repo prints 0.1.3.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-08}
pm 0.1.3 is released only after sprint 98 fixes the release path, from current main, with no git hook skipped; the staged release branch is not committed.
The owner chose to fix the hook first over a one-time --no-verify: the staged branch lacks main's later changes, and a release should never need a skipped hook. Answers `yeeef-agents-9va.102.2`.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- pm 0.1.3 tagged pm-v0.1.3 at 5dbac54 and pinned in 3baf051 (PR #86), through
  sprint 98's tag-before-pin path; pm where in the release worktree prints pm
  0.1.3 launched from that commit.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: pm 0.1.3 is tagged pm-v0.1.3 and pinned through sprint 98's release path, with no git hook skipped; it reaches main when PR #86 merges.

Merged as 0a51c0f (PR #86).

- Commit A `5dbac54` bumps `pm/pyproject.toml` and `pm/uv.lock`; tag `pm-v0.1.3` on origin names it.
- Commit B `3baf051` moves the pin and the Beads hook markers (`pm upgrade`).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- The bump PR is on main with green CI: PR #86 CI green (light, integration, guard, both Go builds); on main once the owner merges it with a merge commit.
- Tag pm-v0.1.3 points at it: met, `git ls-remote origin refs/tags/pm-v0.1.3^{}` = `5dbac54`, the bump commit in PR #86.
- `pm --version` in this repo prints 0.1.3: met on the release branch; `pm where` there prints `pm 0.1.3 this repo's pin at commit 5dbac54…, launched by the pm uv tool (pm 0.1.2)`. On main after the merge.
