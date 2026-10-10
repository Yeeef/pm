---
type: sprint
title: Design pm's own work store to replace Beads
bead: yeeef-agents-9va.86
---

## Goal

> What should be true when this sprint ends, and why now?

A design page fixes pm's own work store, so the build sprint can start without open design questions. The owner chose to replace Beads with a store pm owns, borrowing bd's ideas and possibly Dolt (sprint 36's decision).

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the data model (items, ids, types, status, parents, dependencies, labels, comments, metadata, holders as sessions); storage and sync across clones and machines, with Dolt as the first candidate and how Python drives it; ready and blocked computation; concurrent writes from many sessions; the session context pm prints in place of `bd prime`; the migration path from `bd export`; the bd commands of the boundary map that each get a pm equivalent.
Since the owner chose Dolt and a Go rewrite: a measured spike of Dolt embedded in a Go program (open and load latency per process, concurrent processes, sync through the git remote, binary size), and a design page for pm in Go with the order in which pm's Python modules are ported.
**Out:** writing the store (next sprint); migrating this repo's data (the sprint after).

## Done when

> What evidence will show the goal is met?

- A design page for the work store covers every row of the boundary map's "What pm uses of bd" table with its pm equivalent.
- Each design choice that needs the owner is raised as a decision need and answered, or recorded as an agent decision with its reason.
- A Go program embedding Dolt loads this repo's 466 items in a fresh process, with the latency measured over 10 runs against the 300 ms goal.
- A design page fixes pm's Go architecture and its port order, and the port's sprints are filed.
- The build sprint's tasks are filed with dependencies.

## Design pages

> Where is the detail?

- [Work store: pm's own replacement for Beads](../design/work-store.md)
- [pm in Go](../design/pm-go.md)
- [pm's boundary with Beads](../design/work-layer-bd-boundary.md)

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-07}
Sprint 77's scope adds a measured spike of Dolt embedded in Go and a design page for pm in Go with its port order.
The owner chose Dolt and a Go rewrite; bd embeds Dolt yet lists in 1-2 s, so the 300 ms load goal must be measured before the build, and the port needs a plan before its sprints.
:::

::: decision {source=agent date=2026-10-08}
The work store drops bd's memories, priority and free metadata, derives in progress from an open item with a holder, files a bug as a task labelled bug, refuses a child-id create while the remote is unreachable, lets an open need not block its task, and keeps .beads until the owner removes it.
Each is cheap to reverse and follows the measured use: pm never reads priority (447 of 466 items at 2), all 4 memories have better homes, typed fields avoid Dolt's JSON-cell merge conflict, and .beads holds the only copy of bd's Dolt history.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Loading all 466 items with comments, fresh process median: files 173 ms,
  Dolt sql-server 179 ms, Dolt CLI 292 ms. A single claim write: Dolt server
  36 ms, files with a git commit 90 ms, Dolt CLI 189 ms (N=30, machine load
  33-191 on 8 cores, so p90 is noisy).

- 8 writers x 20 writes on one store: Dolt CLI fails 119 of 160 (database is
  read only), files fail 141 of 160 on git's index.lock; with one flock both
  reach 160 of 160 (files 19.1 s, Dolt CLI 31.5 s); Dolt sql-server 160 of 160
  in 3.0 s.

- Offline edits on two clones: Dolt merges different fields of one item but
  conflicts on different metadata keys (one JSON cell); one JSON file per item
  with a JSON merge driver merges both; all backends conflict on the same
  field and on the same new child id. Dolt push takes 2.2-3.1 s, git push
  0.1-0.7 s.

- Dolt 2.4.2 syncs through a git remote (refs/dolt/data) once the remote has a
  branch; brew has no Dolt bottle for macOS 14 and its source build failed, so
  Dolt came from the release tarball (119 MB binary).

- uv and pm's tool Python on this Mac are x86_64 builds running under Rosetta,
  which likely slows every pm command.

- Owner's motivation, measured: bd list --all --json took 1.16-2.09 s (5 runs)
  and bd export 1.69-2.28 s (3 runs) at load average 70 on 8 cores; pm's
  load() runs both, and 20 pm commands call it (show, task add/claim/close,
  finding add, decision add, sprint open/close, ...), so each pays about 3-4 s
  in bd before its own work.

- Store size after about 380 commits on 466 items: Dolt 14 MB, 3.0 MB after
  dolt gc; files on git 15 MB, 3.6 MB after git gc. bd's own store is 123 MB
  for 596 KB of exported data (2,041 commits), 47 MB after dolt gc on a copy.

- Dolt embedded in a Go binary (dolthub/driver, as bd) loads all 466 items
  with labels, dependencies and comments in a fresh process in 64 ms median,
  68 ms p90 (open 21, queries 2.7, close 14; n>=10, load average 20-50 on an
  8-core M1); 68 ms after 3,000 more commits. A write with DOLT_COMMIT takes
  70 ms in a fresh process.

- bd list --all --json on a copy profiles at 567 ms: about 410 ms in 9 Dolt
  engine opens and closes (one per transaction), 117 ms cleaning a git remote
  cache on close, about 105 ms more process start than a plain Go binary; the
  query itself takes about 36 ms. pm pays the open and close once per process instead of 9 times; whether its close also pays the git-remote cache cleanup, and its process start with Dolt linked in, are not measured.

- The embedded Dolt engine holds an exclusive lock on the store while open,
  readers included; 8 processes x 20 writes lost nothing (160 of 160) with the
  driver's open retry, 7.7 s total, p90 wait 468 ms; a flock gate bounds the
  max wait to 524 ms. A service holding the store open blocks every CLI
  process.

- Embedded Dolt syncs through a git+file remote (refs/dolt/data): push about
  600 ms, no-op pull 180 ms; different cells of one row merge automatically, a
  same-cell edit lands in dolt_conflicts_items with base, ours and theirs. The
  stripped darwin/arm64 binary is 107 MB, needs CGO and -tags gms_pure_go; a
  linux/amd64 build via zig cc is 132 MB, not yet run.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: pm's work store is designed on Dolt embedded in a Go pm, measured at 64 ms for a full load, and the Go port is planned and filed as sprints.

- [Work store: pm's own replacement for Beads](../design/work-store.md): data model, ids, ready, commands, session context, migration and storage on embedded Dolt.
- [pm in Go](../design/pm-go.md): architecture, store sharing, site parity, distribution and the port order.
- Owner decisions: Dolt for the store, and pm rewritten in Go.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Met:** the work-store page maps every row of the boundary page's bd uses to a pm equivalent and every one of its 10 constraints to a fix.
- **Met:** the backend went to the owner as a decision need and was answered (Dolt); the Go rewrite is an owner decision; the agent's own design choices are a sprint decision with reasons.
- **Met:** Dolt embedded in Go loads the 466 items in a fresh process in 64 ms median, 68 ms p90, over at least 10 runs, against the 300 ms goal.
- **Met:** the pm-go page fixes the architecture and port order; the port is filed as sprints 78, 79 and 83 to 91 with their dependencies.
- **Met:** sprint 78's tasks are filed with dependencies; its load-time check is kept.
