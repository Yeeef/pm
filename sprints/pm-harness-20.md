---
type: sprint
title: Faster site reloads
bead: yeeef-agents-9va.24
---

## Goal

> What should be true when this sprint ends, and why now?

Reloading a page on the local site feels instant: today a reload takes 0.75–0.86 s (sprint 14 page, 41 pages rendered on every request), of which `bd list --all --json` alone is 0.46 s.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** measuring where the time goes (Beads read, record parsing, rendering all pages for one request); making a reload fast when nothing changed and still current when something did, for example by caching on the records store state and the Beads data, or rendering only the requested page; keeping the "never stale" guarantee of sprint 11.

**Out:** hosting beyond this machine; changing what pages show.

## Done when

> What evidence will show the goal is met?

- A reload with nothing changed takes under 150 ms, and one right after a `pm` write or a `bd` change under 500 ms, measured with curl on the real store; the numbers are recorded before and after.
- A change still shows on the next reload; the existing freshness test passes, and a test covers the cache or partial render.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-05}
pm serve keys its cache on the record files' text and the Dolt store's manifest bytes and file sizes, rereads Beads in the request only when those moved, and renders only the requested page.
The size fingerprint changed on every bd write tried and on no read, costs 0.2 ms, and a background reread made reloads after a write slower.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Baseline 2026-10-05: three reloads of /sprints/pm-harness-14.html took 0.86,
  0.75 and 0.75 s (41 pages rendered per request); bd list --all --json alone
  took 0.46 s.

- Profile of one pm serve request on the real store (2026-10-05, 42 records,
  129 issues): bd list --all --json 0.48-0.59 s; reading the 42 record files 4
  ms, parsing them 17-31 ms; rendering all 43 pages 0.23-0.25 s (one page 7-13
  ms, the answered-need check 7-10 ms); serving itself negligible. A Dolt
  write grows the chunk journal under .beads/embeddeddolt/<db>/.dolt/noms on
  every bd create, update, label, comment, dep add/remove, reopen and close,
  while bd list and bd show leave sizes and the manifest unchanged (they only
  touch mtimes), so manifest bytes plus file sizes are an exact, 0.2 ms Beads
  change signal; no bd call is cheaper than 0.1 s (bd context 0.09-0.14 s, bd
  count 0.24 s, bd list --limit 1 0.51 s).

- Rereading Beads in a background thread as soon as the Dolt fingerprint moves
  made reloads slower, not faster: a bd write grows the journal every 5-20 ms
  for its whole 0.45-0.56 s run, so the prefetch started mid-command, competed
  with the writer and was outdated at once; reloads right after a bd write
  took 0.89-1.31 s against 0.50-0.53 s without it. Dropped; the request
  rereads Beads itself.

- Reloads before and after the cache (curl on the real store, 42 records, 129
  issues): before, 0.73-0.95 s for any reload (sprint 14 page and index, 7
  loads). After (commit a0cd1f6): nothing changed 7-8 ms (10 loads; index 8-13
  ms); right after a records edit 38 ms (3 loads) and after a pm write (pm
  decision add) 34 ms; right after a bd write 0.51-0.53 s (bd comments add and
  bd update, 8 loads), against bd list --all --json alone at 0.47-0.49 s, so
  the 500 ms target after a bd change is missed by the bd read itself. Other
  agents' bd writes also make the next load reread Beads, as they must.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Partial: an unchanged reload takes 7–13 ms and one after a `pm` write about
35 ms, but one after a `bd` change still takes 0.51–0.53 s, because reading
Beads (`bd list`) alone costs about 0.48 s.

- `pm serve` reuses a rendered page while the record texts and a cheap Dolt
  change signal (0.2 ms to read) are unchanged, and renders only the
  requested page on a change.
- Beads are reread only when the Dolt journal moved; every bd write tried
  (create, update, label, comment, dep, close) moves it.
- A failing record now shows its error on its own page only; `make render`
  still fails the whole site.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Under 150 ms with nothing changed: met, 7–8 ms (index 8–13 ms) over 10
  loads, from 0.73–0.95 s.
- Under 500 ms after a `pm` write or a `bd` change: met after a records edit
  or `pm` write (34–38 ms); missed after a `bd` change (0.51–0.53 s over 8
  loads), bounded by `bd list` at 0.47–0.49 s. A background reread made it
  worse (0.89–1.31 s) and was dropped (sprint decision).
- A change still shows on the next reload, with tests: met,
  `test_serve_reuses_a_page_only_while_records_and_beads_are_unchanged` and
  `test_serve_renders_fresh_on_every_load` (a0cd1f6).
