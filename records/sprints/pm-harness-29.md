---
type: sprint
title: Site pages never wait on writers
bead: yeeef-agents-9va.34
---

## Goal

> What should be true when this sprint ends, and why now?

Loading a page on the site never waits on other sessions' writes. On 2026-10-05 a page took about 15 s after a site reply because it queued behind two sessions writing to Beads and the records store; the owner should never feel that. Revisit how `pm serve` reads, so pages stay fast and current while agents write.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** measure where a request waits (a timing line per request: records-store lock, Beads read or write, render); a design review of the site's read path written into the views-and-site design page, with options such as reading committed snapshots instead of taking the store lock, serving the last good page while a newer one renders and saying so on the page, keeping Beads data from a background refresh, or moving a reply's Beads write off the request; the owner picks; then build it.

**Out:** hosting beyond this machine; changing what pages show.

## Done when

> What evidence will show the goal is met?

- Every request logs its total time and where it waited; the 15 s case is reproduced or explained with these numbers.
- A design decision on the read path, recorded with its options and costs.
- With another session writing continuously, page loads and a reply stay under 500 ms, measured; and a page never shows data older than what it states.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-05}
The owner leaves the site read path's implementation to the agent and holds it to the end goal in sprint 29's frame: page loads and replies stay under 500 ms while others write, and a page never shows data older than it states; the owner reviews the final design page.
The owner cares about the experience, not the mechanism (site reply on 2026-10-05).
Answers `yeeef-agents-9va.34.3`.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Where a page waits (pm serve timing log, own server on :8772, 2026-10-05, 10
  rounds of 2 loads plus a reply per run): idle, a cached page takes 6 ms and
  a reread 0.5-1.9 s, all of it bd list. With bd update looping, every load
  rereads Beads (0.5-2.9 s) and a reply POST took 7.9 s (bd comments add 6.4 s
  waiting on Dolt). With pm finding add looping, loads wait on the store lock
  (up to 2.6 s, since a pm write holds it across its own bd list and commit)
  and the GET after a reply queued 4.0 s behind the reply's background reread
  plus 1.1 s of lock: 7.4 s for reply plus GET. With both writers (the owner's
  case) the reply plus GET took 9.5 s and 5.8 s, loads had median 1.2-1.8 s
  and max 6.5 s, one GET spent 6.3 s in bd list, and one bd update took 20.7
  s: the 15 s is these waits stacked, queue + store lock + bd list + bd
  comments add, each lengthened by embedded Dolt contention.

- The Beads fingerprint (dolt_state) moved 50 times in 30 s while no measured
  writer ran, only other sessions' bd readers (three pm reply wait polls and
  the owner's server): journal.idx flipped between two sizes (1213513 and
  1222374 bytes) and transient nbs_manifest_<n> files came and went in noms/.
  Each move makes the next page load pay a full bd list (0.5-1.9 s), so even
  an idle-looking site rereads often; a fingerprint that ignores these files
  would cut that.

- Site read path built (A+B+D+E), measured 2026-10-05 on the real store with a
  writer holding the store lock across bd list and bd update in a loop plus a
  bd update loop, both on [TEST] issues: before (inline reads under the lock,
  20 loads, 10 replies) page load median 2.7 s / max 11.0 s, reply POST 4.8 s
  / 44.6 s, GET after a reply 4.9 s / 22.2 s. After (snapshot read without the
  lock, 120 loads, 60 replies) page load 2 ms / 30 ms, reply POST 2 ms / 5 ms,
  GET after a reply 1 ms / 6 ms; stated age median 2.6 s, p90 16.2 s, max 19.1
  s, so 20% of loads stated more than 10 s, all of it bd list (up to 16.4 s
  under Dolt contention). With the snapshot read under the lock (B+D+E only)
  the stated age was median 7.3 s, p90 30.1 s, max 35.1 s, which is why A was
  added. No page showed data older than it stated (0 of 60 checks against
  completed bd updates).

- pm serve reply spool, live on the real store: a reply POSTed (303), its bd
  write held by a slowed bd, kill -9, restart: 1 comment, spool empty; a
  second restart: still 1. A real-browser double click sent 1 POST; the form's
  handler keeps the reply id for the same text and makes a new one for changed
  text.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: pages and site replies no longer wait on other sessions' writes,
each page states how old its data is, and no page shows data older than it
states; under heavy load a page can still sit more than 10 s behind, and says
so.

- Pages come from a background snapshot read without the store lock; they
  state "Data as of HH:MM:SS (N s ago)" and update themselves.
- A reply returns at once; the card shows "Saving your reply…", then "Reply
  sent." or the error with the text back in the box.
- Every request logs where its time went.
- Replies are delivered at least once and written once: a fsynced spool
  before the 303, replay at start, and a reply id against duplicates.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Every request logs its total time and where it waited; the 15 s case is
  reproduced and explained: met. Pages queued behind the store lock and
  `bd list` (up to 6.5 s per load, 9.5 s per reply with two writers);
  rendering took 3-7 ms (6f22eaf; finding on this sprint).
- A design decision on the read path, with options and costs: met. The owner
  left the implementation to the agent; the project decision chose B+D+E,
  and A was added after measuring.
- With other sessions writing, page loads and a reply stay under 500 ms and
  no page shows data older than it states: met. 120 loads: 2 ms median,
  30 ms max; 60 replies: 2 ms median, 5 ms max; every finished `bd update`
  before a page's stated time was on it (f650ca3, 27c5346). The stated age
  ran over 10 s on 20% of loads under heavy Dolt contention (median 2.6 s,
  p90 16.2 s), bounded by `bd list` itself.
