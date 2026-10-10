---
type: sprint
title: Release pm 0.1.4
bead: yeeef-agents-9va.111
---

## Goal

> What should be true when this sprint ends, and why now?

Release pm 0.1.4 and pin this repo to it, so the pm service writes the four-theme day summary and the site lists open work that sits in no sprint.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Commit A: `version` 0.1.4 in `pm/pyproject.toml` and `uv lock`; tag `pm-v0.1.4` on A and push it.
- Commit B: `pm upgrade` moves the pin to 0.1.4.
- The PR, and the service restart after the merge.

**Out:**
- Any code change.

## Done when

> What evidence will show the goal is met?

- Tag `pm-v0.1.4` on origin names commit A, and the PR with A and B is on main (merge commit).
- After the merge, the pm service on this clone runs 0.1.4.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: pm 0.1.4 is tagged on origin, and this repo's pin moves to it in the release PR.

Merged as f20bace (PR #91).

- Tag `pm-v0.1.4` names the version bump commit; the pin commit follows it on the same branch.
- 0.1.4 carries the four-theme day summary and the site's list of open work that sits in no sprint.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Tag `pm-v0.1.4` on origin names commit A: **met**. `git ls-remote --tags origin 'pm-v0.1.4^{}'` gives 450e1b1, the version bump commit. The PR with A and B is on main: **met**. Merged with a merge commit as f20bace; `git merge-base --is-ancestor 450e1b1 origin/main` holds.
- The pm service on this clone runs 0.1.4 after the merge: **met**. After `pm service restart`, launchd's process runs `uv tool run --from …@450e1b1… pm service run`, the tagged commit.
