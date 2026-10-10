---
type: sprint
title: Design page schema
bead: yeeef-agents-9va.5
---

## Goal

> What should be true when this sprint ends, and why now?

Design pages share one template of sections, so every design reads the same
way and agents know where things go, while keeping the free-form detail that
makes a design page useful. Now, because the harness page is the only design
page so far and other projects are about to write their own.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the section template (Problem; Goals and non-goals; Constraints and
key facts; Design; Alternatives considered; Prior art, optional; Open
questions), each with a prompt line; the renderer requiring the required
sections of a `type: design` record; `pm design new` creating a page with
every section; applying the template to the harness design page; the rules,
skill and design-page docs that describe design pages.

**Out:** decisions and plans, which stay in project and sprint records;
checks on section order, extra sections or section content; docs
(`pm doc new`), which stay free-form.

## Done when

> What evidence will show the goal is met?

- `make render` refuses a `type: design` record missing a required section,
  and the harness design page renders with a table of contents.
- `pm design new <slug> --title … --project NAME` writes
  `records/design/<slug>.md` with every section and its prompt line; its
  refusals exit non-zero, print why and change nothing; `make test` passes.
- `records/design/pm-harness.md` uses the template's sections, keeping its
  content in its final state.
- `RULES.md`, `SKILL.md`, `status-site/pages.md` and the design page's own
  record-type docs describe the template.
- The owner approved the template through the need raised for it.

## Design pages

> Where is the detail?

- [Record layer](../design/record-layer.md): the design page template (The design page)

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-03}
Design pages have seven `##` sections in this order: Problem, Goals and
non-goals, Constraints and key facts, Design, Alternatives considered,
Prior art, Open questions. Problem, Goals and non-goals and Design always
have content; Constraints and key facts, Alternatives considered and Open
questions are required but may hold "None yet."; Prior art is optional.
These are the owner's candidate sections; requiredness follows his
recommendation, because what we solve, what we aim at and the design itself
are the page, while the other parts may legitimately be empty.
:::

::: decision {source=agent date=2026-10-03}
The renderer checks only that each required `##` section heading is present,
as it does for project and sprint records: not their order, not extra
sections, not their content. The owner wants free-form detail and light
enforcement; a presence check keeps every page navigable for agents at no
cost to authors, and ordering or content checks can be added if pages drift.
:::

::: decision {source=agent date=2026-10-03}
Section headings are plain names without numbers (`## Design`, not
`## 4. Design`), so the check is an exact heading match like other records;
the table of contents already strips numbers, so nothing is lost. Free
subsections live as `###` under Design, which the table of contents lists.
Smaller "Not chosen" notes stay beside the choice they lost to; Alternatives
considered holds the alternatives that need their own explanation.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- The harness design page fit the template without losing content: its eleven
  numbered sections became the seven template sections, with the former layer,
  CLI, views and implementation sections as six ### subsections of Design.
  Only the Plan section, history notes (spike, v1/v2 sprint numbers, a stale
  'planned' label) and the code tree duplicated under Implementation were cut;
  Goals and non-goals and the key-facts list were new, written from the
  project Goal and facts stated elsewhere on the page (555 to 621 lines,
  including the template docs).

- Many smaller 'Not chosen' notes sit beside their choice throughout Design;
  moving them into Alternatives considered would cut each from its context, so
  Alternatives considered holds only alternatives that need their own
  explanation and points to the inline notes.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: design pages share an approved template that the renderer checks
lightly, and `pm design new` creates pages in it.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **`make render` refuses a design record missing a required section; the
  harness page renders with a table of contents:** met. Six refusal tests,
  one per required section; `make render` builds 16 pages.
- **`pm design new` writes every section; refusals change nothing; `make
  test` passes:** met. 136 tests pass, 14 of them for this sprint.
- **The harness design page uses the template:** met. Its 11 numbered
  sections are now the 7 template sections (commit `d9cb33f`).
- **RULES.md, SKILL.md, pages.md and the design page describe it:** met
  (commit `b8d17ba`).
- **The owner approved the template:** met. `yeeef-agents-9va.5.5` answered
  with `pm need respond` (commit `40f65d5`).
