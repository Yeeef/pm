---
type: sprint
title: Drafts across clones, and this repo on the new record layer
bead: pm-c72a.6
---

## Goal

> What should be true when this sprint ends, and why now?

A draft made in one clone shows on every other clone's site, a sprint closed from any clone publishes it, and this repo runs the release that carries the whole record layer. Today drafts live only in their clone: a sprint closed from clone B leaves clone A's drafts tagged to a closed sprint, and an owner's site shows only the drafts of the machine it runs on. This is step 5 of [record-drafts](../design/record-drafts.md), then the project's last step: move this repo's pin.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Per-clone draft refs `refs/pm/drafts/<clone>/<name>`: a clone id made once by `pm init`; the service pushes each draft commit with `--force-with-lease` against the sha it last pushed, flags a lease failure, and fetches others' refs; other clones list them as committed drafts with no session.
- `pm sprint close` publishes the sprint's pages from fetched draft refs too, and deletes a remote ref once all its pages are published.
- A PR moving this repo's pin with `pm upgrade --to` the release that carries steps 1 to 5, merged through `make merge-ready`; a live check on this repo after the upgrade.

**Out:**
- Live (uncommitted) drafts from other machines: only committed drafts travel.

## Done when

> What evidence will show the goal is met?

- A second clone lists the first clone's draft as a version after one follow and one sync each, and a close from the second clone publishes it (a test with two clones of one bare origin).
- A lease failure is flagged, never forced over (a test).
- CI green on the sprint's PR, merged through `make merge-ready`.
- This repo pins the new release; after the upgrade, in a fresh worktree here, `records/` is a workspace, a record edit shows as that worktree's version on the site, and `pm commit` publishes it (live check, numbers in Findings).

## Design pages

> Where is the detail?

- [Record drafts and publishing](../design/record-drafts.md): Draft refs, Publish at sprint close, Increment 5

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
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

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
