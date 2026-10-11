---
type: sprint
title: Trim the tests to what has unique value, then split internal/cli
bead: pm-d2k5.20
---

## Goal

> What should be true when this sprint ends, and why now?

The suite keeps only tests with unique value, and `internal/cli` stops being a god package. An audit of 432 non-live tests found:
- 147 tests have no unique statements.
- 10% of the tests cover 86% of the lines; 20% cover 94.6%.
- 75 tests are bug guards, added along with a fix.

With the trimmed suite as the safety net, `internal/cli` (8.9k LOC, 34% of non-test code) is split by noun, and its Python-port leftovers go.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Apply the audit's cut (owner decision 2026-10-11):
  - keep 145 tests;
  - fold 160 as rows into the kept test each overlaps most, sharing one setup (the merge/sync tests in `internal/work` above all);
  - drop 127.
  - Every bug guard stays.
- Split `internal/cli` by noun.
- Break up the 5 longest functions: `fromIssue`, `cmdUninstall`, `parse`, `cmdSprintClose`, `mergeItem`.
- Replace the hand-rolled argparse emulation with pflag plus explicit checks, where no user-visible output depends on it.
- Retire `pyjson` where no output depends on it.

**Out:**
- The 63 live-eval cases.
- New features.

## Done when

> What evidence will show the goal is met?

- The test count falls by at least 25%.
- Combined coverage (Go plus light harness, measured as in the audit) is at least 97% of the 9,042 statements covered before.
- All 75 bug guards are still present, by name or as a row.
- CI is green.
- No file in `internal/cli` is over 800 lines.
- No function has gocyclo over 30.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-11}
Trim tests by the audit: keep 145, fold 160 into table tests, drop 127; every bug guard stays
A strict 10% keeps 86% of lines but loses 71 of 75 bug guards and most contracts, saving ~10 s; the speed is in folding, not deleting
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Phase 1 (PR #48): non-live tests went from 432 to 164 (-62%): Go top-level
  tests 228 to 83 (go test -list), non-live pytest items 204 to 81
  (--collect-only). 150 MERGE rows are folded as rows (Go t.Run, pytest '#
  folded:' blocks). 73 DROP rows are gone, 46 DROP rows were folded instead
  and 8 kept by name (unique contract, or named by CLAUDE.md, AGENTS.md or
  RACE_TESTS). 10 MERGE rows kept by name: 3 race tests whose targets do not
  match RACE_TESTS, the 2 bench tests,
  TestMermaidLoadsOneExactVersionAtNaturalWidth,
  TestChunksFitTheCapAndAddUpToTheHead, and 3 test_pm tests whose ids clash
  with every target's state.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
