---
type: sprint
title: The pitch deck reads from across a room
bead: yeeef-agents-9va.81
---

## Goal

> What should be true when this sprint ends, and why now?

Each slide of the pitch deck carries only its essentials in type large enough to read from across a room, so a presenter can pitch pm in 10 minutes. Why now: the owner found too many words on every slide on 2026-10-08.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** every slide reduced to a headline, at most three short lines and one visual, in large type; the current detail moved into collapsed speaker notes under each slide; the same 7 slides and three problems; republishing the shared artifact.
**Out:** new content; a different structure.

## Done when

> What evidence will show the goal is met?

- Each slide shows at most about 30 words outside its speaker notes, with a headline at least about 2.5 rem. Expected: counted on the rendered page.
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

done: every slide of the pitch deck shows only a large headline, at most three short lines and one visual, with the detail in collapsed speaker notes; slide 4 leads with the site as the place to read agent outcomes; the shared artifact is at version 4.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Each slide shows at most about 30 words outside its speaker notes, with a headline of about 2.5 rem: met.** On-screen words per slide, counted on the rendered HTML outside `<details>`: 28, 32, 31, 31, 30, 26, 26 (including the "n/7" marker); headlines clamp(1.9rem, 5vw, 2.8rem); records commit 284f3f6; `pm check` rendered all 115 pages; no horizontal scroll at 390 px.
- **The artifact at its link shows the new version: met.** Version 4.
