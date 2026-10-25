---
type: sprint
title: "Rename a project's title: pm project edit, and pm check flags title drift"
bead: pm-d2k5.47
---

## Goal

> What should be true when this sprint ends, and why now?

A project's title can be changed through pm, so the work store and the record never disagree on it: `pm project edit <name> --title "…"` writes both and records the rename as a project decision, and `pm check` names any project whose record `title:` differs from its work-store title. Now, because the poker repo renamed three projects by hand-editing their record headers and the work-store titles stayed stale in `pm show`, sprint parents and the day summary, with nothing reporting the drift.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm project edit <name> --title "…"` with the reason as its body, following `pm sprint edit`: work-store title first, then one records commit with the record's `title:` header and a project decision (source=agent, owner with `--confirmed`); a failed records step puts the old store title back.
- Refuse a title that both the store and the record already hold; accept one only the record holds (the poker repo's three projects). The project's name, its record path and links stay unchanged.
- `pm check` flags a project whose record title differs from its work-store title, naming `pm project edit` as the fix.
- `--help` texts, `prime.md`'s one line, the changelog entry and tests.

**Out:**
- Renaming a project's name (its record path and links).
- Title drift checks for sprints.

## Done when

> What evidence will show the goal is met?

- The harness tests for `pm project edit` pass: store and header both updated, decision added, unchanged title refused, header-only drift accepted, failed records step restores the store title.
- `pm check` on a store with a project whose header and store title differ prints a finding naming the project and `pm project edit`; the PR's CI is green.

## Design pages

> Where is the detail?

- [Sprint lifecycle](../design/sprint-lifecycle.md): Rename a project, `pm project edit`, its cases and refusals.
- [Record layer](../design/record-layer.md): `pm check` reports project title drift, without failing.
- [pm CLI](../design/pm-cli.md): Naming gains `project edit`.
- [Views and the site](../design/views-and-site.md): `covers:` gains `internal/cli/dayactivity/**`, where a day's activity moved.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-25}
pm project edit renames a closed project too, unlike pm sprint edit.
pm check now flags title drift on every project, closed ones included, so a closed project needs a way to fix the finding; a title is display text and changes no outcome a close recorded.
:::

::: decision {source=agent date=2026-10-25}
The design page agent-lifecycle stays as it is for this sprint's code changes.
prime.md only adds pm project edit to the move-or-rename line; the agent lifecycle it describes is unchanged.
:::

::: decision {source=agent date=2026-10-25}
The design page pm-product stays as it is for this sprint's code changes.
prime.md only adds pm project edit to its move-or-rename line; the product design it describes is unchanged.
:::

::: decision {source=owner date=2026-10-25}
The agent merges PR #122 itself, through make merge-ready.
The owner asked in chat: "plz merge it yourself".
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- PR #122 CI green on all jobs, light and integration included. Locally: make
  test 159 passed, 169 skipped; tests/test_pm.py 59 passed, 3 of them new
  (rename with its refusals and rollback; pm check drift then fix; store
  already holds the title); internal/cli and internal/records Go tests ok;
  make lint 0 issues.

- internal/cli sat exactly at its size ratchet (7301 of 7301 lines), so the
  day's activity code moved to internal/cli/dayactivity first; after the
  rebase onto 0.16.0 the package is 7227 lines and the ratchet is lowered to
  that.

- A fresh-context review found no correctness bugs and three low findings, all
  fixed: the drift hint is now runnable as printed, with shell single quotes
  and the body form; it starts on its own line after the design report; the
  case where the store already holds the title has a test.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: `pm project edit` renames a project's title in the work store and its record together, and `pm check` names every project whose two titles differ.

Merged as 9520997 (PR #122).

- `pm project edit <name> --title "…"`: writes the work-store title, then one records commit with the `title:` header and a project decision (agent, or owner with `--confirmed`). A failed records step puts the old store title back. It refuses only a title that the store and the header both already hold, so a header that a hand edit already changed is fixed. Closed projects are allowed. The name, path and links are unchanged.
- `pm check` reports title drift with a pasteable `pm project edit` hint; this is a report, not a failure, so `pm commit` never refuses on it.
- The day's activity code moved to `internal/cli/dayactivity` to keep `internal/cli` under its size cap.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- The harness tests for `pm project edit` pass (store and header both updated, decision added, unchanged title refused, header-only drift accepted, failed records step restores the store title): met. `tests/test_pm.py` 59 passed, with the rename, drift and store-holds-title tests; PR #122's `light` and `integration` jobs pass.
- `pm check` prints a finding that names the project and `pm project edit` when the header and the store title differ, and the PR's CI is green: met. `test_check_names_a_hand_edited_project_title_and_project_edit_fixes_it` asserts the exact line, and `test_check_holds_covers_to_main_and_lists_uncovered_sources` asserts it runs after the design report on its own line. Every job on PR #122 passed.
