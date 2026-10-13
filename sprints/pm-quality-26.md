---
type: sprint
title: A covers pattern for unmerged code does not fail pm check for every session
bead: pm-d2k5.26
---

## Goal

> What should be true when this sprint ends, and why now?

`pm check` passes on main while other sessions' branches are in flight. Today it exits 1 for every session: 27 `covers:` patterns name files that exist only on unmerged branches (PR #62's CLI split, the agents-md evaluation). The records store is shared across branches, and the design-page gate makes a sprint update its page before its PR merges. So a page names a file before main has it, by design. The dead-pattern rule still catches rot, at the point where main must have the file.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm check` reports a covers pattern that matches no tracked file on main, without failing. It keeps failing on a malformed pattern and on a dead `design_sources` pattern.
- `pm sprint close` fails when a page in the sprint's Design pages, or named by its gate, has a covers pattern that matches nothing on main once the sprint's PR is merged.
- Tests, `--help`, and the design pages of the areas changed.

**Out:**
- The patterns themselves; their sessions fix them when they merge.

## Done when

> What evidence will show the goal is met?

- On main today, `pm check` exits 0 and lists the 27 patterns as waiting on unmerged work.
- A harness test: a sprint whose page names a file its merged PR does not add is refused at close, naming the pattern; the same page with a live pattern closes.
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
