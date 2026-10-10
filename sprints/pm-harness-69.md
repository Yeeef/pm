---
type: sprint
title: The pitch deck speaks to the three problems pm solves
bead: yeeef-agents-9va.78
---

## Goal

> What should be true when this sprint ends, and why now?

The pitch deck reads as a story about three problems a reader recognises, in plain language, so a non-specialist sees why pm matters within 10 minutes. Why now: the owner found the first version too technical on 2026-10-07.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** rewriting the 7 slides around the owner's three problems: tracking agent work and its growing context in a structured way; agents collaborating with each other at scale, through structure; agents and humans collaborating; fewer command names and internals, kept mostly to the last slide; republishing the shared artifact at the same link.
**Out:** new pm features; a second deck.

## Done when

> What evidence will show the goal is met?

- The deck has at most 7 slides; the three problems are the motivation, each shown with the answer pm gives; the last slide keeps the bootstrap command for pm-v0.1.2. Expected: checked on the rendered page.
- The artifact at its existing link shows the new version.

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

done: the pitch deck tells pm's story through the three problems the owner named, in plain language, and the shared artifact shows it (version 2).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **At most 7 slides; the three problems are the motivation, each with pm's answer; the last slide keeps the pm-v0.1.2 command: met.** The served page has 7 `<section>` slides and no escaped HTML; slides 2 to 4 each pair a "today" card with a "with pm" card for one problem; command names are gone outside slide 7 and one source citation; `pm check` rendered all 112 pages; screenshots at 1280 px light and dark and 390 px show no overlap or horizontal scroll.
- **The artifact at its existing link shows the new version: met.** Published as version 2 at the same URL; it is shared as "Anyone with the link".
