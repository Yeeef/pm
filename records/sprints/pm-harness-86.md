---
type: sprint
title: "Go port: agent commands, part one (P5)"
bead: yeeef-agents-9va.95
---

## Goal

> What should be true when this sprint ends, and why now?

The record-and-task commands agents use most run on Go pm with the same outputs, refusals and writes as Python pm. It joins the work store core and the records-and-site sprints into the first live-data parity check.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** `show`, `record link`, `commit`, `task add/close/claim/move`, `finding add`, `feedback add`, `doc/design/postmortem new`, `project/sprint open/close`; the static check that each Python refusal string's constant part appears in Go's source, for these commands.

**Out:** decision, action, reply, day and owner-request commands; the work-store-only commands and sync; service and install.

## Done when

> What evidence will show the goal is met?

- These commands' `test_pm.py` scenarios pass with `PM_IMPL=go`.
- Their differential transcripts equal Python's after normalisation.
- Live-data parity on a copy of this clone: every record page, `pm show`, `pm where` and `pm check` equal between Python on bd and Go on the import.
- The sprint's PR is on main.

## Design pages

> Where is the detail?

- [pm in Go](../design/pm-go.md): Tests, Port order and coexistence

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-09}
Go pm lists an item's blockers by id; the live parity compares both sides with blockers in that order, and Python on bd printing another order is an accepted difference until the cut-over.
bd lists blockers in its storage order, which no data defines: of 11 issues with 2 or more blockers, 3 list ascending by id, 6 descending and 2 neither, and one pair lists in both orders, so no stored order could match it.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Shared suite on Go pm after the port: 46 passed, 70 expected failures (110
  before; 40 left the list, 2 new tests added), 186 skipped;
  compare_transcripts: 46 transcripts equal to Python's, 0 differ. Seeding
  Go's store needed fixture ids in the work-store form (demo -> repo-demo:
  CheckID requires <prefix>-<root>), seeds with updated_at/closed_at and
  claimed_by, and the fake bd stamping writes as bd does (create, close,
  claim, update), else exports differed null against stamp. Static refusal
  check (tests/test_go_refusals.py): 155 constant parts (119 distinct) of the
  refusals of 27 ported Python functions, all in Go source.

- Live-data parity on a scratch clone (records branch at 021d42c, bd export of
  564 issues and 285 comments, 2026-10-09): Go's import equals
  tests/work_items.py's mapping on all 564 items, every field. pm where, pm
  check (all 150 pages render) and 26 of 28 distinct pm show commands (top
  level, --sprint for 16 open sprints, --record, 4 of 5 --project) byte-equal,
  Python pm on the export through the fake bd against Go pm on the import.
  Pages: 149 compared, 142 equal and 3 equal after the assignee allow-list
  entry; 4 differ only in the task graph's edge order. Every remaining
  difference has one of two causes: blocked_by order (bd keeps insertion
  order, the work store keeps the blockers sorted, 1 task line in pm show
  --project pm-harness and 4 graphs) and the assignee the work store does not
  hold (5 holders in pm show --json).

- Where Go pm differs from Python pm by the work store's design, and the
  transcripts show it: pm where's Beads line names the work store (one
  placeholder in transcript.py); a records step that fails after an item was
  created closes that item as dismissed (the store has no delete) where Python
  prints a bd delete hint; closed_by is the holder's session, else this
  session; a claim records no host; the work store needed Move to take needs
  (pm task move moves any open non-epic item) and no Dolt commit for a write
  that changed nothing (a claim again within one second failed with 'nothing
  to commit'). Go pm now wraps a usage error's usage line as Python 3.13's
  argparse does (it printed it unwrapped).

- Left for later sprints: (1) blocker order: the work store sorts blocked_by,
  so pm show's '(by …)' list and the task graph's edges come out in another
  order than bd's insertion order; equal sets, no test asserts the order. (2)
  pm where's codex line for an unparsable config carries the TOML parser's own
  message, which differs between BurntSushi/toml and tomllib. (3) pm where's
  service line uses Go's service.Health, which on a clone whose unit runs
  Python pm reports it stale. (4) Root ids minted by Go take the main
  checkout's directory name as prefix; a clone in a directory named otherwise
  would mint another prefix.

- Fresh-context review of PR #90: no correctness bug found. The argparse usage
  wrapping was byte-equal to Python 3.13's for 41 commands at COLUMNS 30 to
  120. Lock order is gate, then records lock, in every command and in the
  service. Fixed in 7b2f16e: a write took the gate and the records lock before
  its own argument checks, so pm task add --title "" waited on the gate where
  Python refuses at once; it now takes them when it first opens the store. pm
  task claim printed a time taken before the store stamped the claim.
  --text-file bodies are now stripped as str.strip does. Left as is: a claim
  that loses a race to another live session prints the store's error, not
  Python's refusal; parse_sections' \s is ASCII-only in Go; the id prefix
  comes from the main checkout's directory name. After the fixes: Go suite 46
  passed, 70 expected failures, 46 transcripts equal; make test 113 passed.

- Live parity rerun after rebasing onto main's natural id order (#92), with a
  fresh bd export (566 issues, 285 comments; records at bfaf733): Go import
  equals the mapper on all 566 items; pm where, pm check and 24 of 26 distinct
  pm show commands are byte-equal, and the other two differ only in blocker
  order and assignee. Of 149 pages, 144 are equal, 1 is equal after the
  assignee entry, and 4 differ only in the task graph's edge order. The Go
  suite now has 47 passed, 70 expected failures and 47 transcripts equal.

- Commit ids after rebasing onto main (#92): the review fix is 7358c71 (was
  7b2f16e); the branch head with CI green on every job is 5f7742d.

- Blocker order, checked before changing the work store as the coordinator
  asked: bd list orders an issue's blockers by its internal storage order,
  which no field of the data reproduces. Of this repo's 11 issues with 2 or
  more blockers, bd list shows 3 ascending by id, 6 descending, 2 neither; by
  when each blocker was added, 3 ascending, 4 descending, 4 neither. The same
  pair lists in both orders under different issues. bd export lists all 40
  issues with several dependencies by id. So keeping insertion order in the
  work store would not make Go match Python, and the store keeps blockers
  sorted by id as text. Accepted difference until the cut-over: pm show's '(by
  …)' list and the task graph's edges list blockers in this order in Go pm and
  in bd's order in Python pm. The live parity corpus hands Python the blockers
  in the store's order (cebf594). The fresh review of this delta confirmed it
  (counts above; three bd list runs identical) and found no bug.

- Live parity rerun with blockers in one order (records f1abbdd, bd export of
  566 issues, 285 comments): Go import equals the mapper on all 566 items; pm
  where, pm check and all 26 distinct pm show commands (top, --json, 5
  --project, 16 --sprint, 3 --record) byte-equal; pages 149 of 149 equal after
  normalisation (constructs 16/16, fixtures 7/7). No task was held at the run,
  so the assignee entry matched nothing. f60f889: frame headings and first
  sentences end at Python's \s (Unicode), with Go tests whose values are
  Python's.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: Go pm runs the agent commands with Python pm's outputs, refusals and writes, and matches Python pm on this repo's live data.

- `show`, `record link`, `where`, `commit`, `task add/close/claim/move`, `finding add`, `feedback add`, `doc/design/postmortem new`, `project/sprint open/close` on Go pm.
- The shared suite seeds Go pm's work store; `pm export` writes every field; a static check finds 155 refusal-text fragments of the ported Python functions in Go's source.
- Merged by the agent session under the owner's project decision that it merges the Go port PRs.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Met:** these commands' `test_pm.py` scenarios pass with `PM_IMPL=go`: 47 passed; the expected-failures list went from 110 to 70.
- **Met:** their differential transcripts equal Python's after normalisation: 47 of 47.
- **Met:** live-data parity on a copy of this clone (566 issues, 285 comments): every record page (149 of 149), every `pm show` (26 of 26), `pm where` and `pm check` equal between Python on bd and Go on the import, with blockers in one order on both sides (sprint decision: bd's own blocker order is storage order).
- **Met:** the PR, [#90](https://github.com/Yeeef/yeeef-agents/pull/90), is on main as `4e0f227`, CI green on `59dec65`.
