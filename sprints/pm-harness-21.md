---
type: sprint
title: Scoped Beads reads in pm
bead: yeeef-agents-9va.25
---

## Goal

> What should be true when this sprint ends, and why now?

`pm` reads only the issues each command needs instead of every issue in Beads, so what it reads stays small as projects grow (context efficiency, owner decision on `yeeef-agents-9va.19.3`).

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** each `pm` command reads and checks only what it touches (owner decision answering `yeeef-agents-9va.25.2`): each command keeps one full `bd` read until `bd` has a subtree query (owner decision answering `yeeef-agents-9va.25.3`); `pm show --sprint` and writes (`check_planned` and the commands that build on `load()`) validate only the records they write plus the issues those reference; the whole-store invariants (an uncited answered need, a draft report on a closed sprint) stay in `make render`, `pm commit` and the site (`cmd_serve`). The two re-reads after a create already narrowed (commit 4455569) carry forward.

**Out:** changing what `make render`, `pm commit` or the site check, or what any command shows; replacing `bd`.

## Done when

> What evidence will show the goal is met?

- Each command makes one full `bd` read and validates only the records it writes or shows plus the issues those reference; whole-store checks run only in `make render`, `pm render`, `pm commit` and `pm serve` (owner decision answering `yeeef-agents-9va.25.3`); tests pin this per command.
- A write that breaks a record it did not touch is still refused by `pm commit` and `pm render` (a test shows it); `make test` and `make render` pass, and main's `pm show` still reads the store.
- Bytes read from `bd` and wall time for `pm show --sprint`, `pm finding add` and `pm task add` are measured before and after on the real store.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-05}
Sending a site reply makes one Beads write and no full re-read: the replied label is dropped (pm reply wait and pm show recognise the owner's comment by its author and time), and after the reply the server patches the cached page instead of re-reading Beads (option D).
Each bd command costs about 0.5 s, so the old three-command reply took over a second; D halves it while "Reply sent" is only shown once the write succeeded.
Answers `yeeef-agents-9va.25.4`.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- A bd call costs about 0.48 s whatever it reads: bd list --all --json
  (184,165 bytes, 140 issues) took 0.49–0.52 s over 5 runs, bd show <id>
  --json (1,056 bytes) took 0.48–0.50 s, and json.loads of the full list takes
  about 1 ms. Process startup, not read size, is the cost.

- Only load_beads() reads the full list; there is no store_reply on main.
  load(), check_planned, cmd_serve, cmd_commit and render.py stay on the full
  list because they render every page to check the whole store, which needs
  nearly every issue. Narrowing them would change what they check, which is
  out of scope.

- Narrowed the re-read after bd create in pm project open and pm sprint open
  to bd show <new id> (184 KB to about 1 KB, same wall time); branch
  scoped-bd-reads, commit 4455569; make test 277 passed, make render 45 pages.

- load() on the real store: bd full read 0.506 s, read_records 0.029 s,
  committed_records 0.041 s, render_pages (whole-store validation) 0.255 s; pm
  show --sprint takes 0.95–0.99 s. bd has no recursive subtree query: bd list
  --parent returns direct children only, so an exact sprint read took 3 bd
  calls (4.5 KB, 0.90–0.93 s) against one full read (186 KB, 0.5 s), and a
  project subtree needs 4+ levels (about 2 s). An id-prefix query is unsound
  because pm task move re-parents without renaming (9va.4.1 and 9va.13.10 sit
  directly under 9va).

- Scoped validation (5eae8b7): load+validate on the real store, 5 runs each,
  before → after: show --sprint 0.72–0.77 → 0.47–0.48 s; finding add 1.01–1.09
  → 0.51–0.54 s; task add 1.01–1.06 → 0.52–0.55 s. The remaining cost is the
  one full bd read (186 KB, about 0.5 s). There is no records-branch
  pre-commit hook; pm commit and pm render are the whole-store checks.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Partial: `pm` writes and `pm show --sprint` now validate only what they touch, which halves their time on the real store, but every command still reads all of Beads once, because `bd` has no single-call subtree query (owner decision answering `yeeef-agents-9va.25.3`).

- Writes validate the records they write plus records of the issues they change and those issues' ancestors; `pm sprint close` and `pm project close` render the closed record first. Whole-store checks run only in `pm render` / `make render`, `pm commit` and `pm serve`.
- `pm project open` and `pm sprint open` re-read only the new epic after `bd create` (184 KB to about 1 KB).
- A site reply makes one Beads write (`bd comments add`, no `replied` label); the next load is served from the patched cache, and a background re-read catches any outside write made meanwhile. "Reply waiting" is now an open request whose comment count exceeds its `picked_up` metadata.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- One full `bd` read per command, scoped validation, whole-store checks only in render, commit and serve, pinned by tests: met. `test_only_render_and_commit_check_records_a_command_does_not_touch` and the per-command tests on branch `scoped-bd-reads`; `make test` 318 passed after merging main (11579e3); PR #28 merged as 5305649.
- A write that breaks an untouched record is still refused at `pm commit` and `pm render`: met, same test (a broken sprint 2 lets writes and `show --sprint demo.1` pass, while `render`, `show --sprint demo.2` and `pm commit` refuse naming it). `make render` 49 pages; `bin/pm show` reads the real store.
- Before and after on the real store (load + validate, 5 runs): met for time, unchanged for bytes. `show --sprint` 0.72–0.77 s to 0.47–0.48 s; `finding add` 1.01–1.09 s to 0.51–0.54 s; `task add` 1.01–1.06 s to 0.52–0.55 s. Bytes read from `bd` stay 184–186 KB per command, by decision; only project and sprint open's second read drops to about 1 KB. Site reply latency is not measured (it would need a real request); by call count it goes from 3 `bd` calls plus a full re-read on the next load to 1 call.
