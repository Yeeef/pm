---
type: sprint
title: The pm service collects Dolt garbage on a schedule
bead: yeeef-agents-9va.90
---

## Goal

> What should be true when this sprint ends, and why now?

The pm service reclaims the Dolt store's dead chunks on a schedule, so the store stays near its live size. bd's store here is 123 MB for 596 KB of exported data (2,041 commits), and a `dolt gc` on a copy brought it to 47 MB. The owner asked for it now; it holds for bd's store today and for pm's own Dolt store later.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** a periodic garbage collection in the pm service on the clone's Dolt store, with its interval, its lock against concurrent bd and pm writes, a timeout, and a log line with size before and after; `pm where` or `pm service status` shows the last run; tests with a fake bd.
**Out:** deleting issues (`bd gc`'s decay phase) and squashing commits (`bd compact`, its compact phase), which rewrite data and the history other clones sync; pm's own Dolt store, which sprints 77 and 78 build.

## Done when

> What evidence will show the goal is met?

- The service runs the collection on its schedule, and the log shows the store's size before and after; on this clone the store shrinks from 123 MB toward the 47 MB measured on a copy.
- After a run, `bd list --all --json` returns the same 466 or more issues as before, and `bd dolt push` succeeds.
- The collection never deletes an issue or squashes a commit: a test asserts the command and flags it runs.

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
