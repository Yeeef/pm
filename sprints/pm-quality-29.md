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

::: decision {source=agent date=2026-10-14}
The design page sprint-lifecycle stays as it is for this sprint's code changes.
Its 'Covers hold on main at the close' section already states the rule as 0.8.6 applies it (a covers pattern of the sprint's); PR #76 only brings pm check's printed line to that wording
:::

::: decision {source=agent date=2026-10-14}
The design page pm-cli stays as it is for this sprint's code changes.
PR #76 changes one sentence of pm check --help to the rule the page already describes; no command, flag or behaviour changed
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: `pm check`'s waiting-pattern line and `pm check --help` now state the close rule as pm 0.8.6 applies it: only the patterns a sprint added or changed while open.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Done when | Met | Evidence |
|---|---|---|
| `pm check` prints the new wording; the harness test asserts it | met | PR #76; `test_check_lists_covers_patterns_that_match_nothing_as_waiting_and_lists_uncovered_sources` passes on the new text |
| Design pages of changed areas updated and listed | met | none changed: the rule's pages (Sprint lifecycle, pm CLI) already state it since sprint 28; only the printed wording changed, waived where the gate asks |
