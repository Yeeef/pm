---
type: sprint
title: Dates on design pages
bead: yeeef-agents-9va.28
---

## Goal

> What should be true when this sprint ends, and why now?

The owner sees how recent each design page is: the site shows each design page's created and last-updated dates, taken from git history of the records store, and orders the overview's design pages by date. Design pages keep their names, since they hold the current state and a date in the name would go stale.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** created and last-updated dates from the store's git history, shown next to each design page's title on the overview and on the page itself; ordering the overview's design list by date; keeping it fast enough for `pm serve`'s cache.

**Out:** renaming design page files; dating docs, which already carry their date.

## Done when

> What evidence will show the goal is met?

- The overview and each design page show created and last-updated dates that match `git log` of the store for that file; a test pins it.
- The overview's design pages are ordered by the date the owner chose (need on this sprint).
- An unchanged reload stays fast (measured against sprint 20's numbers).

## Design pages

> Where is the detail?

- [Views and the site](../design/views-and-site.md): design page dates, the overview's order, and what pm serve keys them on

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-05}
The overview orders design pages by last updated, newest first; both created and last-updated dates are shown.
The overview is read to see what is moving (owner chose option A on the site).
Answers `yeeef-agents-9va.28.2`.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Design page dates cost one git log over design/ plus one git status, 45-47
  ms on the real store (10 pages); pm serve recomputes them only when record
  texts, the store HEAD or the day change, so an unchanged reload stays 7-9 ms
  (index 11 loads, design page 10 loads after the first). git log --follow
  also follows copies (a near-identical new page got an older page's created
  date in a test), so dates follow renames only.

- pm action need reads optional extra context from stdin while holding the
  records-store lock: on 2026-10-05 another session ran it without </dev/null,
  its stdin stayed open, and it held the lock for over 4 minutes, blocking
  every pm write in every session (including this sprint's draft report
  commit) until that process ended. pm should read stdin before taking the
  lock.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: each design page shows its created and last-updated dates from the
records store's git history, and the overview lists design pages by last
update, newest first.

- Dates come from one `git log` over the design pages, following renames; an
  uncommitted page counts as updated today.
- The `pm serve` cache also covers the dates, so an unchanged reload stays at
  7–9 ms; recomputing them costs 45–47 ms after a change.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- The overview and each design page show dates matching `git log` for that
  file, with a test: met. All 10 served design pages matched `git log
  --follow` and `git log -1` on the real store;
  `test_design_pages_show_dates_from_the_stores_git_history` (046018c).
- The overview is ordered by the owner's chosen date: met, last updated,
  newest first (owner decision on 9va.28.2).
- An unchanged reload stays fast: met, 7.6–9.1 ms against sprint 20's 7–13 ms;
  `test_serve_shows_design_dates_after_a_commit_that_changes_no_file`.
