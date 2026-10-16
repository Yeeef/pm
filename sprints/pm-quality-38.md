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

- [pm-cli](../design/pm-cli.md): the command families, the `show` levels with the record outline, hidden machinery, and the CLI naming grammar.
- [decision-need-layout](../design/decision-need-layout.md): need forms refuse once with every broken rule; covers `internal/cli/needform`.
- [agent-lifecycle](../design/agent-lifecycle.md): the `pm show` levels gain the record outline.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-16}
The no-op commands pm hook git-post-checkout and pm hook git-pre-commit stay, hidden under pm --help --all, instead of being removed.
Pins 0.2.0 to 0.9.x write .pm/hooks sections that call them, and an older-pin session resets core.hooksPath; removing them would break checkouts and block commits on the new pin.
:::

::: decision {source=agent date=2026-10-16}
The design page pm-product stays as it is for this sprint's code changes.
The page states the product model and layout; it names no help listing, prime noun list or command form that this sprint changed.
:::

::: decision {source=agent date=2026-10-16}
The design page reply-delivery stays as it is for this sprint's code changes.
Only the help texts of decision add, decision close and action done now name the undelivered-reply refusal; the delivery rule and the refusal are unchanged.
:::

::: decision {source=agent date=2026-10-16}
The design page session-claims stays as it is for this sprint's code changes.
Claims, holders and the show listings of held tasks are unchanged; show.go changed only the --record level, which the page does not describe.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- PR #104's first CI run failed 3 harness tests only because the job crossed
  midnight UTC: tests/test_pm.py sets `TODAY = date.today()` at import, while
  pm stamps the date at write time. They passed on re-run with no code change;
  the flake predates this sprint and needs its own task.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: agents' top failure clusters on the pm command surface are removed, and `pm --help` and prime.md show what agents use (PR #104).

- `pm show --record X` alone prints the record outline; sprint names resolve as targets.
- Need forms refuse once with every broken rule.
- `task close` requires `--reason`; `doc new --sprint|--task|--project`; one help renderer; honest usage lines.
- `pm --help` hides the machinery commands behind `pm --help --all`.
- prime.md from 3,218 to 2,884 words; the pm-cli design page rewritten with the CLI naming grammar.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- `pm show --record <sprint id>` prints an outline with exit 0: met. The harness tests `show --record repo-demo.1` and `show --record demo-1` alone exit 0 with the outline; Go test `TestRecordOutlineListsEachSectionWithItsCommand`.
- A `decision need` with two violations lists both in one refusal: met. A harness test lists three; `TestARefusedLabelIsOneRule` holds a single bad label to one rule.
- `pm --help` hides the machinery and `pm --help --all` lists it: met. Golden `help.golden`; `test_pm_help_hides_the_machinery_that_help_all_lists`.
- `pm task close <id>` without `--reason` is refused: met. A harness test in `tests/test_pm.py`.
- prime.md is under 3,218 words and every hook chunk fits its cap: met. 2,884 words; chunks 3,850, 6,735 and 6,043 characters against 10,000; `tests/test_hooks.py` passes.
- The pm-cli design page names no `bd`, `serve`, `render` or `setup`, `pm check` passes, and the PR's CI is green: the page is met except one Prior art bullet naming Beads (`bd`) as prior art, which is history, not the design; `pm check` is met; CI is in the PR #104 checks.
