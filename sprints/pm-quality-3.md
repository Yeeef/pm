---
type: sprint
title: "pm CI: a PR's slowest check under 5 minutes"
bead: pm-d2k5.3
---

## Goal

> What should be true when this sprint ends, and why now?

A typical pm PR's slowest check finishes in under 5 minutes, so a sprint's PR is ready for review soon after it is pushed. Why now: PR #7's build-and-parity job took 11m29s on Linux and 14m45s on macOS for a prompt-only change, most of it compiling.

Moved from pm-harness sprint 110 on 2026-10-10.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A Go build cache that is refreshed (today setup-go saves only on a new go.sum, so the cache is from 2026-10-09).
- The release build test run only when the release build can change (release/, go.mod, go.sum) and on tags.
- The race tests and the shared suite split into parallel jobs.

**Out:**
- Making the tests themselves faster (internal/work's 81 s Dolt tests).
- Dropping the macOS runner.

## Done when

> What evidence will show the goal is met?

- Two PRs after the change, neither touching release/ nor go.sum: each one's slowest check under 5 minutes, read from gh pr checks.
- A PR touching release/ still runs the release build test, and passes.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-10}
Fix the GC journal flake by building with Dolt b130ee82ebe9 (dolthub/dolt#11312), guarded by TestDoltHoldsTheJournalPruneFix; pm does not serialize GC with sync.
The race is inside Dolt journal (any journal re-creation during a prune hits it, not only the fetch); upstream fix fails-before/passes-after 20/20; a GC that waits on sync deadlocks, since DOLT_GC waits for sessions mid-statement (host.go:328-332).
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- From pm-harness sprint 110: PR #7 Linux build-and-parity, 11m29s, from the
  job log: setup 0:30; build, vet and Go tests 1:56 (internal/work 81 s);
  -race tests 3:47 (tests 6 s and 39 s, the rest compiling with -race); parity
  0:20; make test-go-suite 1:51; release build test 2:56 (release/build.sh
  twice). The setup-go cache hit its primary key and logged 'not saving
  cache'; the cache dates from 2026-10-09, the last go.sum change. That the
  -race compile is uncached is inferred from those dates, not measured.

- Before the change (pm go run 38050732408, PR #13, one job per target): Linux
  9:31 = setup 0:32 (886 MB setup-go cache) + go-build 0:11 + vet and tests
  1:48 (internal/work 81.9 s) + race internal/service 3:01 + race
  internal/work 0:48 (40.0 s of tests) + release build test 3:01; macOS 10:16
  = setup 1:04 (891 MB cache) + go-build 0:25 + vet and tests 2:58
  (internal/work 133.3 s) + race 2:14 + 1:10 (56.2 s of tests) + release build
  test 2:17. The setup-go cache hit its primary key (the go.sum hash) and was
  never saved after 2026-10-09: the non-race build was warm (go-build 11 s),
  the -race compile cold (internal/service: 6.5 s of tests in a 3:01 step). Go
  test results were cached across runs too (pyjson "(cached)").

- The UpdateGCGen fatal is a race inside Dolt, not a GC outliving its test
  (Host.GC is synchronous; cleanup is Close then RemoveAll). In Dolt
  a6690826d767 (pinned), ChunkJournal.PruneTableFiles
  (store/nbs/journal.go:350-361) protects the journal only if a writer exists
  when it starts; a concurrent write that bootstraps the journal
  (journal.go:512, outside pruneMu) gets its file deleted by the prune
  (file_table_persister.go:373-420), its writes land in an unlinked file, and
  the next GC panics in dropJournalWriter (journal.go:455). PR #16 race job
  114223542469 reproduced it in 10.7 s with a DATA RACE report: dolt_fetch (pm
  sync) bootstrapJournalWriter vs GC PruneTableFiles. Upstream fix
  dolthub/dolt#11312 (b130ee82ebe9, 2026-07-17): its regression test fails
  20/20 on the pinned Dolt, passes 20/20 (and 5/5 -race) on the fix. pm level,
  no local repro in 330 runs (0 failures) before; after the bump, -run GC
  -count=30 ok (93.2 s), -race -run GC -count=5 ok (20.6 s). Production
  exposed: the service GCs while commands and syncs write.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
