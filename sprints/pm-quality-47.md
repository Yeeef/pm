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
