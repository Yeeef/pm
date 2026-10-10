---
type: sprint
title: pm decision add takes the decision and its reason as flags
bead: yeeef-agents-9va.76
---

## Goal

> What should be true when this sprint ends, and why now?

`pm decision add` takes the decision and its reason as two required flags, `--decision` and `--reason`, so an agent can write a well-formed decision from `--help` alone. Today the command reads both from stdin, as "the decision on its first line, its reason on the next". Only `pm prime` states that layout. An agent that has not read it, or misreads it, writes a malformed block. A required flag makes the input structural: argparse refuses a missing part before anything is written.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** `--decision` and `--reason` on `pm decision add`, both required, each one non-empty line; the command no longer reads stdin; its `--help`, `prime.md` and the hints in other refusals and pages updated; tests.

**Out:** the `::: decision` record format (unchanged: decision line, then reason line); `pm task move`, which still takes its reason on stdin; records already written.

## Done when

> What evidence will show the goal is met?

- `uv run --project pm pm decision add --help` shows `--decision` and `--reason` as required, and `prime.md` names them; no text in `src/pm/` tells an agent to pipe a decision on stdin.
- A test adds a decision with the flags and checks the written block; a test shows a missing flag, a multi-line value and a value starting with `:::` are refused with nothing written.
- `make test` passes, and the PR's CI is green.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-07}
Keep sprint 67's change: pm decision add takes --decision and --reason as flags, beside sprint 70's decision need flags.
The owner chose keep: both commands then take their structured parts as flags.
Answers `yeeef-agents-9va.76.3`.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- pm decision add now takes --decision and --reason, both required, one line
  each, and reads no stdin; the ::: decision block it writes is unchanged.
  make test: 70 passed, 35 skipped; PR #70 CI: light, integration and guard
  pass. The 4 refusal cases (missing --reason, multi-line --decision, blank
  --reason, --reason starting with :::) each exit non-zero with no bd write
  and no store change.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: `pm decision add` takes the decision and its reason as required flags, `--decision` and `--reason`, and reads no stdin, so the input is structural.

Merged as d5eed01 (PR #70).

- PR #70: the flags, a one-line check on each, `--help`, `prime.md` and the "once the owner answers" hints; the `::: decision` block is unchanged.
- The design page [pm CLI](../design/pm-cli.md) describes the flags.
- Agents in other repos get the flags at the next pm release; repos pinned to 0.1.2 keep the stdin form until then.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Met:** `pm decision add --help` shows `--decision TEXT --reason TEXT` as required; `prime.md` names both flags. A grep of `src/pm/` for "decision add", "stdin", "as the body", "first line" and "next line" finds only `pm task move`'s own stdin reason, which is out of scope.
- **Met:** `test_decision_add_need_closes_need_and_records_decision` adds a decision with the flags and checks the written block. `test_decision_add_refuses_a_malformed_part_and_writes_nothing` covers a missing `--reason`, a multi-line `--decision`, a blank `--reason` and a `--reason` that starts with `:::`. In each case there is no bd write and no change to the store.
- **Met:** `make test`: 70 passed, 35 skipped (the integration tests). PR #70 CI on 3a75d54: the light, integration and guard jobs pass.
