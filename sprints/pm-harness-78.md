---
type: sprint
title: "Go port: work store core (P3)"
bead: yeeef-agents-9va.87
---

## Goal

> What should be true when this sprint ends, and why now?

Go pm has its work store: the Go package `work` on embedded Dolt holds every item with its fields, ids, readiness and the store gate, and imports and exports this repo's bd data without loss. It is the first step after the Go skeleton on the port's critical path, and every later parity run seeds Go through its importer.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- The Go package `work` on embedded Dolt (dolthub/driver): schema and its version, typed items and validation, ids and minting, ready and blocked, the store gate.
- `--import-bd` from `bd export`, and `pm export`.
- The round trip on this repo's real `bd export`.

**Out:** the work-store commands, sync, merge rules and the child-id compare-and-swap (work-store commands and sync sprint); `pm init`, doctor and the service on the store (install sprint); moving Python pm's `bd` calls, since Python pm keeps bd until the cut-over; migrating this repo's data (cut-over sprint).

## Done when

> What evidence will show the goal is met?

- The round trip on this repo's real `bd export` passes every check of the work store page's migration: item count, each id's type, status, parent, blockers, title and description, comment count per item, every need's `raised_by` and `delivered`.
- The importer's items equal those of the neutral-test sprint's Python mapper on the same export.
- Go tests for the schema, ids, ready and blocked and the gate pass, derived from the work store page's definitions.
- Loading every item of this repo's imported data from the work store takes at most 300 ms, against 3–4 s through bd (`bd list --all --json` plus `bd export`), measured over 10 runs.
- The sprint's PR is on main.

## Design pages

> Where is the detail?

- [Work store: pm's own replacement for Beads](../design/work-store.md)
- [pm in Go](../design/pm-go.md): Port order and coexistence

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-07}
Sprint 78's Done when gains a load-time check: pm show spends at most 300 ms loading every item, against 3-4 s through bd today.
The owner named bd's slow bd list --all --json, which 20 pm commands pay, as a motivation for replacing it; files measured 173 ms.
:::

::: decision {source=agent date=2026-10-08}
Sprint 78 is rescoped to the Go port's work store core: the Go package work on embedded Dolt (schema, ids, ready, gate), --import-bd and export, and the round trip on the real bd export; the 300 ms load check stays. Its sub-tasks for merge, sync and the compare-and-swap and for the agent work commands moved to the work-store commands and sync sprint (89), and init/doctor/service setup to the install sprint (91), by bd update --parent because pm task move refuses sub-tasks.
The owner chose embedded Dolt and a Go rewrite over the files design this sprint was framed on; the pm-go page's port order puts these pieces in separate sprints.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Round trip on this repo's real bd export (539 items, 2026-10-08): pm init
  --import-bd then pm export equals the bd export on every check of the work
  store page's migration: 539 items, 270 comments, 56 blockers, 174 needs
  (type, status, parent, blockers, title, description, comment count,
  raised_by, delivered). Mapped in 10 ms, imported in 474 ms as one Dolt
  commit. The Go importer's items equal tests/work_items.py's on all 539,
  absent fields aside. Command: PM_BD_EXPORT=<bd export > file>
  PM_BD_RECORDS=$(pm where records) go test -tags gms_pure_go -run RoundTrip
  ./internal/work/ and the same env for test_go_parity.py -k import_bd.

- Load benchmark: a fresh-process pm export (config check, gate, open, load
  and check of every item, JSON) of the 539 imported items, optimized build
  (cgo, -trimpath -tags gms_pure_go -s -w, 107,687,730 bytes on darwin/arm64),
  10 runs on the 8-core M1 at load average about 23: median 75 ms, min 70, max
  84; goal at most 300 ms. Same machine and minutes, bd list --all --json plus
  bd export, 10 runs: median 2,096 ms, min 1,195, max 2,492.

- The real bd export had two open bugs with no parent (yeeef-agents-2se from
  2026-10-06, yeeef-agents-915 created 2026-10-08), which the import refuses
  as the page says. Both are pm test flakes; moved under the pm-harness
  project with bd update --parent. bd keeps letting such items in, so the
  cut-over must expect the import to refuse new ones.

- dolthub/driver v2.2.0 panics reading an ENUM column (rows.go:233 asserts
  uint16), so the schema's enums are CHECK constraints on VARCHAR columns; the
  schema still refuses a value outside the enum (tested). The driver also
  needs the go directive at 1.26.2, and grpc v1.79.3 with x/net v0.54.0 does
  not build (http2.TrailerPrefix), so go.mod takes the spike's x/net v0.58.0
  and grpc v1.83.1.

- pm export leaves a field out where it does not apply (the skeleton's Item
  JSON), while tests/work_items.py writes every field with null; the
  importer-vs-mapper check compares with null, empty strings and empty lists
  dropped. Transcript parity (P5) compares exports directly, so it needs one
  shape: either the mapper drops absent fields or pm export writes them.

- Fresh-context review of PR #84: no partial-write, round-trip or CI issue;
  fixed in 8fd8007: comments written in one second came back in random order
  (now a pos column, bd's order kept on import), a changed comment was
  silently not written (now refused), a timed-out gate wait was not logged
  (now timeout=1), plus three nits. Left as is, for the page or a later
  sprint: a sprint's number is checked against its record's file name, not the
  record's title (records' titles carry no 'Sprint N:'); a bug labelled human
  imports as a need labelled bug, as work_items.py does, with no row on the
  page; the Python mapper accepts in_progress without claimed_by and open with
  claimed_by, which Go refuses (the real export has neither); pm task ready
  orders by sprint number across projects. After the fixes: real round trip
  539/270/56/174 equal, mapper agreement on all 539, pm export median 70 ms
  (max 99) over 10 runs.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: Go pm's work store on embedded Dolt imports and exports this repo's bd data without loss and loads every item in 70 ms, against 2,096 ms through bd.

Merged as ce6569f (PR #84).

- `internal/work`: the typed schema with a schema version, invariants checked on every read and write, ids and minting, ready, blocked and cycles, and the store gate with its wait log.
- `pm init --import-bd` and `pm export`, Go-only commands until the cut-over.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Met:** the round trip on the real bd export passes every migration check: 539 items, 270 comments, 56 blockers and 174 needs equal (`TestRoundTripOfARealExport`).
- **Met:** the Go importer equals the Python test mapper on all 539 real items, with null and empty fields dropped (`test_import_bd_agrees_with_the_python_mapper`).
- **Met:** Go tests for the schema, ids, ready and blocked, and the gate pass: `go test -tags gms_pure_go ./internal/work`.
- **Met:** loading every item takes at most 300 ms over 10 runs: `pm export` in a fresh process, optimized build, 539 items, median 70 ms, max 99 ms; `bd list --all --json` plus `bd export` take a median 2,096 ms on the same machine.
- **Met:** the PR, [#84](https://github.com/Yeeef/yeeef-agents/pull/84), is on main as `ce6569f`.
