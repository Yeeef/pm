---
type: sprint
title: Drop main's records/ copy; keep the records/ link
bead: yeeef-agents-9va.85
---

## Goal

> What should be true when this sprint ends, and why now?

`main` no longer carries a `records/` copy. pm installs no copy Action, no PR guard, no pre-commit records guard and no sparse checkout, and each worktree keeps its `records/` link to the store. Why now: the owner chose option C in sprint 56, because no machine reads main's copy and it alone causes those four mechanisms.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Remove the copy and guard workflows and the pre-commit records guard from pm's installer and from this repo; delete `records/` on `main` in the same PR.
- Stop making the sparse checkout in `setup_clone`; undo it in worktrees that have it; update `pm where` and `pm doctor`.
- Point `pm/AGENTS.md`, `prime.md` and the records-store design page at the `records` branch or the site in place of main's copy.
- Tests for the new setup.

**Out:**
- The `records/` link and `additionalDirectories`, which stay.
- The store's location.

## Done when

> What evidence will show the goal is met?

- `git ls-tree origin/main records` prints nothing once the PR is on main, and `.github/workflows` holds only `pm-tests.yml`.
- In a new worktree, `pm init` makes the link and no sparse checkout: `git config core.sparseCheckout` is unset and `pm doctor` reports nothing.
- The pm test suite passes in CI.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-07}
Moved yeeef-agents-9va.85.1 to yeeef-agents-9va.65: The owner asked to build option C inside sprint 56, which made the choice, rather than in a new sprint.
Sprint 76 is voided with no work done.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

voided: the owner asked to build option C in sprint 56, which made the choice; its one task moved there, with no work done here.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

None met here; every item moved into sprint 56's Done when.
