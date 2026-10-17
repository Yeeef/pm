---
type: sprint
title: Design-page index at sprint open and first claim
bead: yeeef-agents-9va.135
---

## Goal

> What should be true when this sprint ends, and why now?

An agent starting work sees the design pages that hold its area's current state, and can find earlier sprints, decisions, docs and work items on a topic in one command. Today it drills down one project and finds anything sideways only by thinking to grep `records/`, while the design-page check runs only at the PR and the close. Design: [Work context](../design/work-context.md).

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm sprint open` and the first `pm task claim` a session makes in a sprint print the design-page index (title, slug, link), as the design page gives.
- `pm search <terms…> [--kind] [--project] [-n]`: BM25 over record sections, decision blocks, design `###` sections, docs, postmortems and work-store task and need titles, one capped line per hit.
- `prime.md`: the two clauses of the design page's Procedure; `--help` texts and goldens; the new files in the page's `covers:`.

**Out:**
- Marking decisions superseded, and printing decisions at claim.
- A hook that injects records per prompt; semantic search; a persistent index.
- Searching a worktree's drafts workspace.

## Done when

> What evidence will show the goal is met?

- Go tests: the index prints on a session's first claim in a sprint and not on its second; `pm sprint open` prints the project's pages; search ranks a seeded unit with the query terms first and caps output at `-n`.
- On this repo's store, `pm search` for three topics with a known prior sprint (claim history, design covers, owner-request judge) lists that sprint's record or decision in its top 5, and one run takes under 1 s (wall time recorded in Findings).
- The live check in a scratch worktree: one claim prints the index, a second claim in the same sprint does not.
- The `prime.md` rules still fit the hook chunk cap (`tests/test_hooks.py` passes).

## Design pages

> Where is the detail?

- [Work context](../design/work-context.md): the design-page index at sprint open and first claim, `pm search`, and the procedure in `prime.md`

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-17}
Moved yeeef-agents-9va.135.2 to yeeef-agents-9va.136: The owner split the work into two sprints: the design-page index stays in sprint 125, pm search moves to sprint 126.
The two are independent and ship as separate PRs.
:::

::: decision {source=agent date=2026-10-17}
Renamed the sprint from "Work context: design-page index at claim and pm search" to "Design-page index at sprint open and first claim": The owner split the sprint: pm search moved to sprint 126.
This sprint now ships only the design-page index, so its title names only that.
:::

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
