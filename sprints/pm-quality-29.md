---
type: sprint
title: pm check's waiting-pattern line states the close rule as it is
bead: pm-d2k5.29
---

## Goal

> What should be true when this sprint ends, and why now?

`pm check`'s line about covers patterns waiting on unmerged work says `pm sprint close` refuses a sprint whose page still names a file missing on the remote's main. Since pm 0.8.6, the close holds a sprint only to the patterns it added or changed while open. The line must say that, so an agent does not wait on a close that would pass.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- The waiting line's wording in `pm check`, and its test.

**Out:**
- The rule itself.

## Done when

> What evidence will show the goal is met?

- `pm check` in this repo prints the new wording, and the harness test asserts it.
- Every existing design page whose area this sprint changes is brought to the new state and listed in Design pages.

## Design pages

> Where is the detail?

None yet.

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
