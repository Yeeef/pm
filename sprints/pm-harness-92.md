---
type: sprint
title: Show a main checkout that is behind on the site
bead: yeeef-agents-9va.101
---

## Goal

> What should be true when this sprint ends, and why now?

The site shows when this clone's main checkout is behind the remote main branch, as it shows a push that needs attention. Hooks, rules and the ~/.claude links read the main checkout, so a stale one runs old pm rules and nobody sees it.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** a flag beside the push flags (`push.flags` in pm/src/pm/push.py, shown on the site through cli.py's site state, and in `pm service status`) that names how many commits the main checkout is behind `<remote>/<main>` and the `git pull --ff-only` command that fixes it; a main checkout off the main branch is flagged as such.
**Out:** the service pulling main itself (the owner declined that); fetching more often than the service already does.

## Done when

> What evidence will show the goal is met?

A test with a main checkout one commit behind its remote shows the flag on the site and in `pm service status`, with the count and the command; an up-to-date checkout shows no flag; `make test` and the PR's CI pass.

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

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
