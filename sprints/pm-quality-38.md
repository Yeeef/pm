---
type: sprint
title: "Simplify the pm command surface: wave 1 plus help and prime"
bead: pm-d2k5.38
---

## Goal

> What should be true when this sprint ends, and why now?

The audit [pm command surface: quality audit and simplifications](../docs/2026-10-16-pm-command-quality-audit.md) found the friction concentrated in a few commands, plus drift and machinery noise in help and prime.md. The owner chose wave 1 plus the help and prime proposals (2026-10-16). When this sprint ends, the observed top failure clusters are gone and an agent's surface (top-level help, prime.md) shows what agents use.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- #1 `pm show --record X` alone prints the record's outline; ids and names resolve like paths.
- #2 `decision need`, `action need` and `need edit` check every rule and refuse once with the full list.
- #3 Remove the no-op `hook git-post-checkout` and `hook git-pre-commit` (after checking `pm upgrade` strips their sections), replace `doc new --bead` with `--sprint/--task`, and render the 9 store commands' help with the one renderer.
- #4 `task close` requires `--reason`.
- #6 `pm --help` hides the machinery leaves behind `pm --help --all`.
- #7 Thin prime.md to full procedures for the most-used commands; move inlined refusal lists to `--help`.
- #9 Rewrite the pm-cli design page to the final state, with the CLI naming table.
- Golden files, `test_pm.py` refusal assertions, hook chunks and the changelog entry.

**Out:**
- #5 scope-flag renames and #8 one need-close verb (they reopen pm-quality 21 and pm-harness 13).
- #10 the grammar CI check (it follows #5).

## Done when

> What evidence will show the goal is met?

- `pm show --record <sprint id>` prints an outline, and exit 0 is shown in the PR.
- A `decision need` with two rule violations lists both in one refusal (a harness test).
- `pm --help` lists no hook, prime, push, sync, export or day command, and `pm --help --all` lists them (golden).
- `pm task close <id>` with no `--reason` is refused (a harness test).
- prime.md's word count is lower than 3,218 and every hook chunk fits its cap (`tests/test_hooks.py`).
- The pm-cli design page names no `bd`, `serve`, `render` or `setup`; `pm check` passes; the PR's CI is green.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-16}
The no-op commands pm hook git-post-checkout and pm hook git-pre-commit stay, hidden under pm --help --all, instead of being removed.
Pins 0.2.0 to 0.9.x write .pm/hooks sections that call them, and an older-pin session resets core.hooksPath; removing them would break checkouts and block commits on the new pin.
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
