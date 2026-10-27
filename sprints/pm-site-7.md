---
type: sprint
title: Project page shows the owner's open needs
bead: pm-bfli.7
---

## Goal

> What should be true when this sprint ends, and why now?

A project's site page shows the open decision, action and question needs of every sprint and task under it, as a sprint page shows its own, so the owner sees what waits on them from the project page. The owner asked for it.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- The project page renders the "Sessions ask you", "Decisions await you" and "Actions await you" cards above its Decisions section, from the work store.

**Out:**
- Any change to the sprint, day or index pages, or to the cards themselves.

## Done when

> What evidence will show the goal is met?

- The site golden test for a project page with an open need under one of its sprints shows the need's card above Decisions, and `make test` passes.
- A project page served live by a scratch pm service shows an open action need of one of its sprints.

## Design pages

> Where is the detail?

- [Views and site](../design/views-and-site.md): the Project page row now names the await-you sections.

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

done: a project's page shows the open question, decision and action needs of everything under it, above its
Decisions, as a sprint page does. PR #123.

- `projectBody` renders the cards with `cardsFor` and `awaiting`, the sprint page's code.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Golden test: met. `go test -tags gms_pure_go ./internal/site -run Golden -update` adds the three sections to
  `projects/demo.html` (one question, two decision and two action cards, from its sprints and the project) above
  Decisions; `make test`: 159 passed, 169 skipped.
- Live: met. A scratch clone's running pm service served `projects/demo.html` with an action need raised on sprint 1
  as a card with a reply form, between "Actions await you" (offset 5793) and Decisions (offset 6986).
