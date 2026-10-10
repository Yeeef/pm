---
type: sprint
title: Changelog in the delivery report
bead: yeeef-agents-9va.20
---

## Goal

> What should be true when this sprint ends, and why now?

A sprint's Outcome reads as a verdict plus a short changelog: the done, partial or voided sentence first, then optionally bullets of what shipped, so the owner sees what changed without reading Against "Done when".

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the Outcome prompt line, RULES.md and the skill wording; a test that bullets after the verdict render and stay out of the Beads close reason.

**Out:** changing Against "Done when", which keeps the evidence.

## Done when

> What evidence will show the goal is met?

- A sprint record whose Outcome has a verdict sentence and bullets renders, and `pm sprint close` puts only the verdict sentence in the close reason; a test covers it.
- The prompt line, RULES.md and the skill describe the verdict-plus-bullets shape.

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

Done: a sprint's Outcome is a verdict sentence followed, optionally, by
bullets of what shipped, and only the verdict becomes the Beads close reason.

- The Outcome prompt, RULES.md and the skill describe the shape.
- Every sprint record's Outcome prompt line is updated.
- A test pins that bullets render and stay out of the close reason.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- An Outcome with a verdict and bullets renders, and `pm sprint close` puts
  only the verdict in the close reason: met,
  `test_sprint_close_keeps_changelog_bullets_out_of_the_close_reason`
  (81f0bd7); `first_para` already did this, so no close code changed.
- The prompt line, RULES.md and the skill describe the shape: met (81f0bd7;
  records 1a3c391).
