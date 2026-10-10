---
type: sprint
title: Incident postmortems
bead: yeeef-agents-9va.3
---

## Goal

> What should be true when this sprint ends, and why now?

Incidents that cost a sprint more than a day are written up and linked from
the project, so the rule that would have caught them is not lost.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** a postmortem record type (timeline, cost, root cause, what changed, what
would have caught it earlier), how it links to sprints and projects, and how
it renders.

**Out:** writing postmortems for past poker-ai incidents.

## Done when

> What evidence will show the goal is met?

- A postmortem record type exists (`records/postmortems/<date>-<slug>.md`,
  sections timeline, cost, root cause, what changed, what would have caught
  it earlier), created by `pm postmortem new`, checked by `make render` and
  linked from its sprint and project pages; tests cover it.
- One real postmortem is written with it: the version-skew breakage of
  2026-10-04, when needs closed by newer pm made pm on main refuse
  everywhere.
- RULES.md says when a postmortem is due.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-04}
The first postmortem (version skew, 2026-10-04) enters the store only after the postmortem code is on main; until then it waits as a draft outside the store.
pm on main and on every other branch refuses a record whose type it does not know, so committing it now would repeat the very incident it describes.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- The version-skew breakage happened a third time: need 9va.13.9 from the 13.6
  live check was dismissed with a note ('Dismissed: live check') on the main
  agent's own instruction, and main's pm only exempts a bare 'Dismissed'; it
  blocked every pm write on main until re-dismissed bare. The postmortem (task
  9va.3.2) should include it.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: incidents get a postmortem record (`pm postmortem new`), checked by
`make render` and listed on their sprint, project and the overview.

- Postmortem type: timeline, cost, root cause, what changed, what would have
  caught it earlier.
- RULES.md: a postmortem is due for an incident that cost more than a day or
  broke other sessions or the owner's view.
- The first postmortem: [version skew broke pm on main](../postmortems/2026-10-04-version-skew-breakage.md),
  committed once this code reached main.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- The postmortem record type, `pm postmortem new`, the render check and
  the links exist, with tests: met (90bef8d);
  `test_postmortem_new_writes_every_section_and_pages_list_it` and the
  refusal tests.
- One real postmortem written: met. The version-skew postmortem (four
  occurrences on 2026-10-04) is in the store (df410cb).
- RULES.md says when a postmortem is due: met (90bef8d).
