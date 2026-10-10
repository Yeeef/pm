---
type: sprint
title: The pitch deck shows its key points on screen
bead: yeeef-agents-9va.83
---

## Goal

> What should be true when this sprint ends, and why now?

Each slide of the pitch deck shows its key supporting points on screen, between the first version's walls of text and the room version's three lines, so the deck works both presented and read alone. Why now: the owner found the room version hid too many details on 2026-10-08.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** each slide keeps its large headline and one visual and gets back 4 or 5 short on-screen points; speaker notes keep only sources, jokes that need reading and install detail; republishing the shared artifact.
**Out:** new content; a different structure.

## Done when

> What evidence will show the goal is met?

- Each slide shows roughly 40 to 60 words outside its speaker notes, with the headline and visual as before. Expected: counted on the rendered page.
- The artifact at its link shows the new version.

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

done: every slide of the pitch deck shows its key points on screen with its large headline and one visual; with 7 slides now a soft target, two crowded slides were split, giving 9; the shared artifact is at version 5.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Each slide shows roughly 40 to 60 words outside its speaker notes: met.** Counted on the rendered HTML outside `<details>`, including headline and visual text: 54, 55, 60, 55, 64, 55, 57, 46, 59; slide 5's card carries about 20 of its 64. Records commit b2f6cef; `pm check` rendered all 117 pages; no horizontal scroll at 390 px.
- **The artifact at its link shows the new version: met.** Version 5.
