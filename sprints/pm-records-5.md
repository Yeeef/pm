---
type: sprint
title: Design pages publish at sprint close
bead: pm-c72a.5
---

## Goal

> What should be true when this sprint ends, and why now?

A design page's edits reach the published records only when its sprint closes, with its code on main, so a page never describes code before the code merges, and two sprints' drafts of one page meet at the close where the closing session can resolve them. Since sprint 4 a design page is tagged with its sprint in the workspace's draft commit but still published at `pm commit`. This is steps 3 and 4 of [record-drafts](../design/record-drafts.md), with the owner's rule that the close publishes and `--discard` drops a page's draft.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm commit` keeps design pages in the workspace's draft commit, tagged with their sprint, and stops publishing them; the tag survives a rename on `records`, or the untagged path becomes a conflict.
- `pm sprint close` publishes every page drafted for the sprint, from every workspace and `drafts/*` branch, in one `records` commit, after its existing checks; an overlap refuses the close naming the commit.
- `pm sprint close --discard <slug> --reason "…"`, recorded as a sprint decision.
- `pm check` holds published covers patterns to main; the waiting list and the close's covers hold retire.
- `pm where` lists `drafts/` branches whose worktree is gone.

**Out:**
- Draft refs across clones (step 5).

## Done when

> What evidence will show the goal is met?

- A scratch sprint with design-page drafts in two worktrees: the published page is unchanged until the close, then the close publishes both drafts in one `records` commit (a test and a live check on a scratch clone).
- A page another sprint published first refuses the close, naming that commit; after the agent resolves and commits in its workspace, the close passes (a test).
- `--discard` drops the draft and records the sprint decision (a test).
- `pm where` lists a `drafts/` branch whose worktree is gone (a test).
- CI green on the sprint's PR, merged through `make merge-ready`.

## Design pages

> Where is the detail?

- [Record drafts and publishing](../design/record-drafts.md): Publish at sprint close, Covers, Increments 3 and 4

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
