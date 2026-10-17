---
type: sprint
title: "pm search: find prior sprints, decisions and design sections by topic"
bead: yeeef-agents-9va.136
---

## Goal

> What should be true when this sprint ends, and why now?

An agent can find earlier sprints, decisions, design sections, docs and work items on a topic in one command, before it frames a sprint. Today it finds anything outside its project's drill-down only by thinking to grep `records/`, which returns lines with no status, date or link and misses work-store titles and dropped tasks. Design: [Work context](../design/work-context.md), pm search and Procedure. Split from sprint 125, which ships the design-page index.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm search <terms…> [--kind] [--project] [-n]`: BM25 over record sections, decision blocks, design `###` sections, docs, postmortems and work-store task and need titles, one capped line per hit, as the design page gives.
- `prime.md`: the open-a-sprint clause (search before writing the frame); `--help` text and golden; the new files in the design page's `covers:`.

**Out:**
- The design-page index (sprint 125).
- Marking decisions superseded; semantic search; a persistent index; searching a worktree's drafts workspace.

## Done when

> What evidence will show the goal is met?

- Go tests: a seeded unit holding the query terms ranks first; `--kind` and `--project` filter; output stops at `-n`; no hit prints the no-match line and exits 0.
- On this repo's store, `pm search` for three topics with a known prior sprint (claim history, design covers, owner-request judge) lists that sprint's record or decision in its top 5, and one run takes under 1 s (wall time recorded in Findings).
- The `prime.md` rules still fit the hook chunk cap (`tests/test_hooks.py` passes).

## Design pages

> Where is the detail?

- [Work context](../design/work-context.md): `pm search` and the open-a-sprint clause of the procedure
- [pm search](../design/pm-search.md): the units, tokenizer, match, ranking and output of `pm search`

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-17}
This sprint also adds the index line "prior work: pm search <topic>" that sprint 125 left out until search shipped.
The pm search design page puts it in the release that ships the command; sprint 125 kept it Out only because search did not exist yet.
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
