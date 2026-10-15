---
type: sprint
title: Audit the pm command surface and propose simplifications
bead: pm-d2k5.37
---

## Goal

> What should be true when this sprint ends, and why now?

pm has grown command by command for weeks. This sprint steps back and measures the quality of the `pm` command surface itself — its size, consistency, how agents actually use it, where it refuses or confuses them — against the designs and decisions that shaped it, and ends with a doc of concrete, ranked simplifications that later sprints can pick up.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Inventory of every `pm` noun, verb and flag, from the built binary's `--help` tree.
- Reading the CLI-related design pages, sprint records, decisions and the pm feedback doc.
- Measured usage evidence: which commands agents run, which they get wrong, which refusals fire.
- A doc record with findings and ranked proposals (merge, remove, rename, make consistent).

**Out:**
- Implementing any proposal: each becomes its own sprint after the owner picks.
- The site, the service internals, the record format.

## Done when

> What evidence will show the goal is met?

- A doc record under pm-quality exists with: the command inventory in numbers, the evidence sources, and at least five ranked proposals, each with its reason, its cost and the decisions it touches.
- `pm check` passes on the store.

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

- The pm command tree grew from 39 nodes (10-07) to 81 (10-15): 63 leaves, 48
  distinct flags. Of 3,768 agent invocations over 11 days, 6.7% failed and
  1.5% were syntax guesses; the top 20 of 65 commands used carry 91% of
  invocations.

- `pm show --record X` without `--section` failed 41 of 42 times, the largest
  single failure cluster; `decision need` failed 30% and `action need` 25%,
  mostly guards refusing one rule per retry. A scope is named 5 ways, a need
  closes 4 ways, and `task close` still defaults its reason to "Done".

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: the audit doc measures the pm command surface, its usage and its drift from the design intent, and ranks ten simplifications.

- [pm command surface: quality audit and simplifications](../docs/2026-10-16-pm-command-quality-audit.md).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- A doc record under pm-quality with the inventory, the evidence sources and at least five ranked proposals, each with reason, cost and decisions touched: met. The doc holds 63 leaves, 48 flags and the growth from 39 to 81 nodes; 3,768 invocations from 252 transcript files; 105 command decisions; ten proposals, four marked as needing an owner decision.
- `pm check` passes: met. "all 242 pages render; a design page covers every tracked source file" (one covers pattern waits on another sprint's unmerged work).
