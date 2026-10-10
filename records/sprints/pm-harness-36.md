---
type: sprint
title: Fork or replace bd so pm is one product
bead: yeeef-agents-9va.41
---

## Goal

> What should be true when this sprint ends, and why now?

The owner decides, from evidence, whether pm forks Beads, replaces it, or stays layered on it, so that pm can become one product with its own context layering and CLI rather than a layer working around bd's.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** what pm uses of bd today and where bd's context, CLI and storage constrain pm; the options (stay layered, fork bd, replace bd with pm's own store) with the cost of each, including migration of existing Beads data and following bd upstream; a recommendation and the owner's decision.
**Out:** carrying out a fork or replacement (later sprints, if chosen); packaging pm as it stands (sprint 33), beyond feeding its design.

## Done when

> What evidence will show the goal is met?

- A design page lists what pm depends on in bd and compares the three options with their costs.
- The owner's decision is recorded as a project decision.
- If the decision is to fork or replace, the follow-up sprints and their tasks are filed.

## Design pages

> Where is the detail?

- [pm's boundary with Beads](../design/work-layer-bd-boundary.md)

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- pm calls bd only as a subprocess and reads only JSON: 38 call sites, 14
  distinct subcommands, out of the about 120 that bd --help lists; pm show
  makes 2 bd calls in 3.18 s wall, 0.7 s of it the full bd export pm needs for
  comment bodies.

- bd upstream is about 1.01 M lines of Go (405 k non-test), with 9 final
  releases and 11 breaking-change entries from 2026-04 to 2026-09; the repo
  pins no bd version.

- bd export keeps issues, labels, dependencies, comments and metadata for all
  450 issues, but drops Dolt history and audit events; a replacement could
  migrate through it, untested for round-trip fidelity.

- Of the 10 bd constraints mapped, 5 (comment export, Dolt fingerprinting,
  profile conflict, triple session context, hook ownership) have a bd
  mechanism pm does not use yet: PRIME.md, the events journal, script hooks.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: the owner chose to replace Beads with pm's own work store, from a design page that maps pm's use of bd and costs three options.

- [pm's boundary with Beads](../design/work-layer-bd-boundary.md): 14 bd subcommands, 10 constraints, options A, B and C costed.
- Project decision: pm replaces Beads with its own store, borrowing bd's ideas, possibly on Dolt.
- Follow-up sprints 77 (design the store), 78 (build it) and 79 (migrate this repo, remove bd), chained in that order.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Met:** a design page lists what pm depends on in bd and compares the three options with their costs: [pm's boundary with Beads](../design/work-layer-bd-boundary.md).
- **Met:** the owner's decision (option C, replace) is a project decision in [pm-harness](../projects/pm-harness.md), answering the sprint's decision need.
- **Met:** the follow-up sprints 77, 78 and 79 are open with six tasks and their sprint dependencies.
