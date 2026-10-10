---
type: sprint
title: A site that is always current
bead: yeeef-agents-9va.11
---

## Goal

> What should be true when this sprint ends, and why now?

The owner opens the local site and always sees the current state of records and Beads, with no command to run first and no stale page. A full render takes 0.75 s today (18 pages), so freshness is cheap.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** how the local site stays fresh (for example re-rendering on page load or on change), how it is started and kept running, and the docs that describe it.

**Out:** hosting the site beyond this machine; redesigning the pages.

## Done when

> What evidence will show the goal is met?

- After a `pm` write or a `bd` change, reloading the open page shows it with no command run; checked for real with a sprint close.
- Starting the site is one documented step, or none if it runs on its own.
- RULES.md, CLAUDE.md and the pm-harness design page no longer tell anyone to run `make render` to see changes.

## Design pages

> Where is the detail?

- [Views and the site](../design/views-and-site.md): how the site is served (the Running row under Implementation)

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- pm serve renders all pages for every HTML request in 0.69 s (curl time_total
  for one sprint page, 21 pages), so a reload shows a pm write or bd change
  with no render step; style.css is served from disk without a render.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: `make docs` serves the site rendered afresh from the records store and
Beads on every page load, so the owner never runs a command to see the
current state.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- A `pm` write or a `bd` change shows on reload with no command run: met.
  On the live server a `pm finding add` and the close of `9va.11.1` both
  appeared on the next reload, and this sprint's own close turned its pill
  from RUNNING to DONE on reload (curl against `pm serve`, no render run);
  `test_serve_renders_fresh_on_every_load`.
- Starting the site is one documented step: met. `make docs` runs
  `pm serve`, which listens on 127.0.0.1 only (commits f6fcaf4, 3b5c08a); a
  failed render shows the error page instead of stale HTML.
- RULES.md, CLAUDE.md and the pm-harness design page no longer tell anyone to
  run `make render` to see changes: met (f6fcaf4; design page fabd8f2).
  `make render` remains as the check before commits.
