---
type: sprint
title: The pm service pulls main after a merge
bead: yeeef-agents-9va.97
---

## Goal

> What should be true when this sprint ends, and why now?

When a reviewed PR merges, the pm service fast-forwards the clone's main checkout itself. A worktree-isolated session cannot run git on the main checkout, so today that step falls to the owner.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the service runs `git pull --ff-only` in the main checkout when it sees a review's merge, only while that checkout is on the main branch; the merge message says the pull's outcome and names the command only when it did not happen.
**Out:** pulling on any other trigger; touching a main checkout on another branch or with a diverged history.

## Done when

> What evidence will show the goal is met?

The integration test for a merged review shows the main checkout at the merge commit, and the pushed message says it was fast-forwarded; a main checkout on another branch is left alone and the message names the command; `make test` and the PR's CI pass.

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

voided: the owner declined the sprint before any work started; nothing shipped.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- None met: the owner declined the sprint before any work started.
