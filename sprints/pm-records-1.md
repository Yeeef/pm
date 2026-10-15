---
type: sprint
title: "Design the record layer: per-sprint drafts, publish at merge"
bead: pm-c72a.1
---

## Goal

> What should be true when this sprint ends, and why now?

The pm-records design page states the record layer's target model, so that the build sprints after it have one agreed design to implement. Why now: on 2026-10-14 two open sprints (PR #89, PR #90) edited the same 8 design pages in one shared working tree, and the owner rejected a per-file version check in `pm commit` as the first step: it refuses the second writer without separating the two sessions' edits, and pm cannot see hand edits. The model needs a private copy per writer and git's three-way merge, and that choice shapes every later sprint.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A design page for the project covering:
  - where a session's or sprint's record edits live (per sprint or per session);
  - which records publish at `pm commit` and which at the PR's merge;
  - merge and conflict rules;
  - how the site shows published records and labelled drafts, and how a broken draft stays off the published page;
  - what the Stop hook's uncommitted-records check becomes;
  - how sync, generated records and setup fit.
- Prior art and alternatives, including the per-file version check and why it was not chosen.
- The build sprints that follow, as increments with their checks.
- Bringing `records-store.md` and `record-layer.md` to the code's current state (the store path, `pm init`, the pm service, no copy on main), so the new design starts from a true baseline.

**Out:**
- Any code change.
- Changes to the work store.

## Done when

> What evidence will show the goal is met?

- The design page exists, validates with `pm check`, and answers each of the five open questions in the starting analysis, or names why one stays open.
- `records-store.md` and `record-layer.md` name no `.records`, `bin/pm setup`, Beads sync or GitHub copy to main.
- The owner has reviewed the design and confirmed or redirected the model; their answer is recorded as a project decision.

## Design pages

> Where is the detail?

- [Record drafts and publishing](../design/record-drafts.md): two candidate models for drafts and publishing, shadow records branches (A) and design pages in the code tree (B), with their comparison and increments

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-14}
Sprint 1 is a design sprint; the design page works out two models in full, shadow records branches (A) and design pages in the code tree (B), and the owner picks one at review.
The per-file version check in pm commit was dropped as a first step: it refuses the second writer without separating the two sessions edits, works per file not per hunk, and pm cannot see edits made with file tools. The choice between A and B shapes every build sprint, so the owner wants both worked out before deciding.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: the record layer's design is agreed: the owner chose Model A, and the record-drafts page states it in its final state, with every version of a page readable on the site.

- [Record drafts and publishing](../design/record-drafts.md): published view at a ref, per-worktree workspaces, follow, journal publish at `pm commit`, design pages published at their PR's merge, draft refs, the version bar, four build increments with checks; B and B′ under Alternatives.
- [Shared records store](../design/records-store.md) and [Record layer](../design/record-layer.md) at the code's current state, the baseline the build starts from.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- The design page exists, validates and answers the five open questions: met. `pm check` renders all 232 pages; the page's section "The starting analysis's open questions" answers each of the five for Model A.
- `records-store.md` and `record-layer.md` name no `.records`, `bin/pm setup`, Beads sync or GitHub copy to main: met. A grep for those terms matches only the Alternatives row of the retired copy on main.
- The owner reviewed the design and the answer is a project decision: met. Need pm-c72a.1.3.1 answered "A"; the project record holds the owner decision. The owner added the per-version view, now goal 5 of the page.
