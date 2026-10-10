---
type: sprint
title: pm decision add says to quote its flags in single quotes
bead: yeeef-agents-9va.82
---

## Goal

> What should be true when this sprint ends, and why now?

`pm decision add`'s `--help`, `prime.md` and the hints that name it tell agents to give `--decision` and `--reason` in single quotes, as `pm decision need` already does. In double quotes the shell runs a backtick code span as a command, and decisions often hold code spans. Today these texts show `--decision "…"`, which leads agents into that.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the `pm decision add` description, the two "record the answer" hints in `cli.py`, and the `prime.md` line.

**Out:** any change to how the flags parse; other commands' quoting.

## Done when

> What evidence will show the goal is met?

- `pm decision add --help`, `prime.md` and both hints show `--decision '…'` and say single quotes; a grep of `src/pm/` finds no `--decision \"` or `--decision "`.
- `make test` passes, and the PR's CI is green.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: `pm decision add`'s help, `prime.md` and both hints tell agents to give `--decision` and `--reason` in single quotes, as `pm decision need` does.

Merged as d5eed01 (PR #73).

- PR #73, stacked on PR #70; no parsing change.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Met:** the help, `prime.md` and both hints show `--decision '…'`; a grep of `src/pm/` finds no double-quoted `--decision`.
- **Met:** `make test`: 82 passed, 35 skipped. PR #73 CI: light, integration and guard pass.
