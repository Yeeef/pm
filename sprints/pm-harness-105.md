---
type: sprint
title: The site's change check survives a records file that git rewrites mid-walk
bead: yeeef-agents-9va.115
---

## Goal

> What should be true when this sprint ends, and why now?

The pm service's per-second change check (`servedSite.Stamp()` in Yeeef/pm `internal/cli/serve.go`) no longer fails when a record file disappears between the directory walk and its read, so a page viewed while the records sync rebases shows the records, not "error: open …: no such file or directory". Found in sprint 103: a scratch test that checked out between two commits of 200 record files saw 182 of 523 `Stamp()` calls fail. It has not been seen live (0 failed refreshes in 107 Go-service rebuilds across two clones), but every remote records change that rewrites a file opens the window.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** `Stamp()` skips a record file that vanished between `WalkDir` and `ReadFile`, as `work.Fingerprint` does for noms files; a test in Yeeef/pm that rewrites record files with git checkouts while calling `Stamp()`; a Yeeef/pm PR.

**Out:** the same race in Python pm (`src/pm`); the store lock or the refresh's retry path; the service-held store redesign (sprint 104).

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

::: decision {source=agent date=2026-10-10}
Moved yeeef-agents-9va.115.1 to pm-d2k5.2: The owner moved pm's quality work (code, architecture, tests, feedback) to its own project, pm-quality, on 2026-10-10.
This sprint's open work continues in pm-quality sprint 2.
:::

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
