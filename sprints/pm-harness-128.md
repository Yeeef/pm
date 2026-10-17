---
type: sprint
title: "Search index RFC: pm search at scale"
bead: yeeef-agents-9va.138
---

## Goal

> What should be true when this sprint ends, and why now?

A design for `pm search` that stays fast and exact as the records and the work store grow, with measured performance and resource costs, so the build-or-wait call rests on numbers rather than guesses. Today every run rebuilds the index from a full scan ([pm search](../design/pm-search.md)). That costs about 0.1 s at 2 MB and grows linearly. This sprint writes the RFC, a design page with the scan as its first step, and builds no product code.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A design page for a persistent, incrementally updated search index: where it lives, how it stays exact against the records branch and the work store, the data structures, the query path, persistence and restart, drafts workspaces, and a semantic (hybrid) layer as a later stage.
- Measurements from throwaway prototypes in a scratch directory: scan versus index at 1×, 10× and 100× this store (build time, memory, on-disk size, query latency, cost of an incremental update), and the store's growth rate from history.
- Stages from today's scan to the full design, each with the measured threshold that triggers it.

**Out:**
- Product code, and any change to `pm search`'s output or semantics.
- Choosing an embedding model or provider.

## Done when

> What evidence will show the goal is met?

- The design page exists, `pm check` passes, and every number on it comes from a measurement in Findings, with the command, the machine, and the corpus size.
- The page gives, for each stage, the corpus size or latency that triggers it, and the memory and disk it costs at 10× and 100×.

## Design pages

> Where is the detail?

None yet.

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

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
