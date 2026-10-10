---
type: sprint
title: Today summary as a bullet list
bead: yeeef-agents-9va.62
---

## Goal

> What should be true when this sprint ends, and why now?

The day page's Today summary reads as a short bullet list instead of a paragraph, so the owner scans what shipped and what waits on them at a glance (owner request, 2026-10-07).

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the summarize prompt asks for 2–4 bullets (still ASD-STE100, no ids: what shipped first, owner actions last); the day page renders them as a list; the index's day list stays readable (first bullet or a joined line).
**Out:** changing what goes into the digest.

## Done when

> What evidence will show the goal is met?

- A real scheduled run writes a bulleted summary and the day page shows a list; checked on the live site.
- make test passes.

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

Done: the Today summary is 2–4 short bullets, what shipped first and owner actions last, shown as a list on the day page.

Merged as 7c8e13a (PR #53).

- The summarize prompt asks for Markdown bullets, one ASD-STE100 sentence each, no ids (PR #53, 7c8e13a).
- The day page renders a list; the index and `pm show` join the bullets with " · " or show the first; paragraph summaries still render.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- A real scheduled run writes a bulleted summary and the day page shows a list: met. The 01:07 run wrote four bullets for 2026-10-07 (first: "Day pages build daily from records with no hand-written work needed by owner."); the served day page has them as `<ul><li>…`.
- make test passes: met, 394 passed, 7 skipped; a test renders a fake bulleted answer as `<ul>` on the day page and a joined line on the index.
