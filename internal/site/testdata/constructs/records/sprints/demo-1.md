---
type: sprint
title: First
bead: demo.1
---

## Goal

> What should be true when this sprint ends, and why now?

Ship the *parser*, because the site needs [tables](../design/parser.md#tables) and `code`.

Second paragraph.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the parser.

**Out:** styling.

## Done when

> What evidence will show the goal is met?

- `make test` passes.
- The page renders &amp; links.

## Design pages

> Where is the detail?

- [The parser](../design/parser.md)

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-02}
Use goldmark.
It is CommonMark.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Parsing takes 3 ms.

::: result {title="Render time"}
| Pages | Time |
|---:|:--|
| 126 | 0.4 s |
:::
Reading: fast enough.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
