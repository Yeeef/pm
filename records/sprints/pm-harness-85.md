---
type: sprint
title: "Go port: records and site (P4)"
bead: yeeef-agents-9va.94
---

## Goal

> What should be true when this sprint ends, and why now?

Go pm parses, checks and renders every record the way Python pm does, so the agent commands and the service have records and pages to build on. It needs no Dolt: it reads items through the store interface, fed by the neutral-test item mapper.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `records`: record parse, sections, blocks, templates, checks.
- `store`: find the `records` store, its lock, atomic multi-file writes, commit.
- The site: the goldmark renderer and its extensions, page templates, the normalised-HTML parity corpus and its allow-list.
- `pm check`.

**Out:** the service that serves the pages; the commands that write records; the Dolt store.

## Done when

> What evidence will show the goal is met?

- Every record page of this repo and of the test fixtures is equal between Python and Go after normalisation, with items from the neutral-test mapper for both.
- The allow-list of accepted differences is reviewed by the owner in the PR.
- `pm check` on this repo's store gives the same result on both.
- The sprint's PR is on main.

## Design pages

> Where is the detail?

- [pm in Go](../design/pm-go.md): Site, Tests

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Page parity, Go against Python after site.Normalise, items from the
  neutral-test mapper on both sides: fixtures 6 of 6 pages equal, constructs
  fixture 15 of 15, this clone's records and Beads (snapshot clone,
  PM_PARITY_LIVE=1, local only) 143 pages, 137 equal and 6 equal only after
  the one allow-list entry: the task graph's bd assignee (' · yeeef'), which
  the work store does not hold. pm check equal on 23 cases (22 broken records
  with their error text, and live: 143 pages).

- goldmark and markdown-it-py differed on one construct in the corpus: a table
  row shorter than the header gets padding cells, aligned by markdown-it and
  unaligned by goldmark. A goldmark AST transformer (cellAlign) fixes it, so
  the allow-list holds no renderer difference.

- This clone's Beads holds 2 open issues with no parent (yeeef-agents-915,
  yeeef-agents-2se). The work-store import (work_items.py) refuses them, so
  the live corpus leaves them out; no page shows them. They block the bd
  import at the cut-over (P3, P10) unless they get a parent or are closed.

- YAML 1.1 header typing and yaml_str quoting match pyyaml on 349 values
  (every header value in the 3 corpora plus edge cases). pyyaml refuses values
  that go.yaml.in/yaml/v3 accepts: a tab in a plain scalar, '=', '<<' and an
  explicit tag such as '!a'. Python's yaml_str raises ValueError on an invalid
  date such as 2026-13-45, where Go quotes it.

- The shared suite on Go pm (make test-go-suite): 3 passed, 110 xfailed and
  180 skipped, out of 293 tests. A test main added during the sprint
  (test_task_claim_refuses_in_the_main_checkout_and_says_how_to_make_a_worktree)
  failed CI's merge run until it joined the expected-failures list.

- After the review the parity corpus renders in Pacific/Auckland on both sides
  (PARITY_TZ); CI runs in UTC and could not catch a UTC date where a local one
  belongs. Results in that zone: fixtures 7 of 7 pages equal, constructs 16 of
  16, live 145 pages: 141 equal and 4 equal after the assignee entry; pm check
  23 cases equal. Against that corpus, the Go side under TZ=UTC fails: 10 of
  15 compared constructs pages equal.

- The fresh review found 2 store bugs, both fixed in 47c2312. The store lock's
  descriptor leaked into child processes: a child saw fds 0-5, not 0-4, while
  the lock was held. Apply took an unreadable record for new: it overwrote it,
  and on a failed commit Restore deleted it. Accepted differences from Python:
  cards list needs newest first, where bd lists by priority, then newest;
  items carry no priority. Summary-JSON error texts and some ISO and YAML
  forms also differ (NaN, 2026-W40-1, +05:99, !!int).

- The Linux pm go job failed 2 of 4 runs on PR 83, each time on an
  intermittent exit 128 from the records worktree setup in a test_go_parity.py
  fixture, a different test each time; both passed on rerun. Branch
  fix-pm-test-flakes adds the command's stderr to find the cause.

- Go pm check now reads the embedded work store (#84), and its result equals
  Python's on this repo. A fresh bd export (550 items, 277 comments) went
  through pm init --import-bd into a scratch store in 0.79 s. Go's pm check on
  a clone of the records branch then printed all 146 pages render in 0.20 s;
  Python's pm check on the store printed all 146 pages render in 3.1 s. Live
  parity, with yeeef-agents-2se and -915 back now that they have parents: 146
  pages, 143 equal and 3 equal after the assignee entry. Fixtures were 7 of 7
  equal, constructs 16 of 16, and the 23 pm check cases all equal. CI on
  f9a146a was green on the first run.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: Go pm parses, checks and renders every record as Python pm does, and its `pm check` reads the embedded work store.

Merged as 4d9c35b (PR #83).

- `internal/records`, `internal/store`, `internal/site`, and `pm check` on the Dolt work store.
- The parity corpus, the HTML normaliser, the allow-list of reviewed differences, and a fixed parity time zone (Pacific/Auckland).
- `make test-go-suite`: the shared suite on Go pm against an expected-failures list that only shrinks (3 pass, 110 expected to fail today).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Met:** every page of this repo and of the fixtures is equal after normalisation, with the same items for both: fixtures 7 of 7, constructs 16 of 16, live 146 pages (143 equal, 3 equal after the allow-list).
- **Met:** the owner reviewed the allow-list in the PR, whose review asked for its approval, and merged it: 1 entry, `assignee`, in `pm/internal/site/testdata/parity-allow.txt` (the work store has no assignee).
- **Met:** `pm check` gives the same result on both for this repo: Go on a store imported from a fresh bd export (550 items) and Python both print `all 146 pages render`, in 0.20 s and 3.1 s; all 23 corpus cases equal, error texts included.
- **Met:** the PR, [#83](https://github.com/Yeeef/yeeef-agents/pull/83), is on main as `4d9c35b`.
