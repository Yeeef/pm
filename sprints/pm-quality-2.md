---
type: sprint
title: The site's change check survives a records file that git rewrites mid-walk
bead: pm-d2k5.2
---

## Goal

> What should be true when this sprint ends, and why now?

The pm service's per-second change check (`servedSite.Stamp()` in Yeeef/pm `internal/cli/serve.go`) no longer fails when a record file disappears between the directory walk and its read, so a page viewed while the records sync rebases shows the records, not "error: open …: no such file or directory". Found in pm-harness sprint 103: a scratch test that checked out between two commits of 200 record files saw 182 of 523 `Stamp()` calls fail. It has not been seen live (0 failed refreshes in 107 Go-service rebuilds across two clones), but every remote records change that rewrites a file opens the window.

Moved from pm-harness sprint 105 on 2026-10-10.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** `Stamp()` skips a record file that vanished between `WalkDir` and `ReadFile`, as `work.Fingerprint` does for noms files; a test in Yeeef/pm that rewrites record files with git checkouts while calling `Stamp()`; a Yeeef/pm PR.

**Out:** the store lock or the refresh's retry path; the service-held store redesign (pm-harness sprint 104).

## Done when

> What evidence will show the goal is met?

- The new test fails on Yeeef/pm main (at least one `Stamp()` error during the checkouts) and passes with the fix (0 errors), and `go test -tags gms_pure_go ./internal/cli/ ./internal/service/` passes.
- The PR is on Yeeef/pm's main.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- --text=A git-checkout race test (40 checkouts between two commits of 200
  record files, the second also dropping a directory of 20) failed on main in
  every run: Stamp() failed 44, 38 and 33 times and records.Texts 45, 36 and
  45 times of 144-148 calls each; the first error was the dropped directory's
  ReadDir (open …/gone: no such file or directory). With the fix: 0 and 0 in 5
  of 5 runs (131-134 calls each), 0.57 s per run.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
