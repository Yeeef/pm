---
type: sprint
title: One pm feedback doc per repo
bead: pm-d2k5.5
---

## Goal

> What should be true when this sprint ends, and why now?

A repo has exactly one pm feedback doc, whatever project an entry is about, so feedback is read in one place. Today `pm feedback add --project NAME` writes one doc per project (`<date>-<project>-feedback.md`); after pm-harness split into pm-quality, pm-site and pm-codex on 2026-10-10 this repo already has two docs with the same title "pm feedback", and would have four.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm feedback add` appends to the repo's one feedback doc, creating it on first use; `--project` becomes optional and, when given, tags the entry (with `--sprint`/`--task` as now).
- The doc's name and header: a doc record ties to a bead or a project today, and needs a date in its name; settle how a repo-level doc validates (decision recorded in this sprint).
- `pm show` and `pm show --project` point at the one doc; the `--help` text, `prime.md`'s feedback line and the tests change with it.
- Merge this repo's two docs (`2026-10-07-pm-harness-feedback.md`, `2026-10-10-pm-quality-feedback.md`) into the one doc, entries in time order, each tagged with its project.
- CHANGELOG entry under [Unreleased], with the upgrade step for a repo that holds several feedback docs.

**Out:**
- Migrating other repos' feedback docs automatically (the changelog names the hand step).
- Any change to what a feedback entry holds besides the project tag.

## Done when

> What evidence will show the goal is met?

- A harness test runs `pm feedback add` against two different projects (and once with no `--project`) and finds all three entries in one doc, each tagged; `pm show` and `pm show --project` both link that doc.
- In this repo, `ls records/docs | grep feedback` prints one file holding all 15+ entries of the two current docs, and its site page renders.
- The PR is on main with CI green.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-10}
The repo-level feedback doc is a doc record at the fixed path records/docs/pm-feedback.md: type doc, title "pm feedback", date of first use, and neither bead nor project in its header; the record checks exempt exactly that path from the date-in-name and project rules, and fail any other doc titled "pm feedback", so a repo holds at most one feedback doc.
A fixed path makes a second doc impossible for pm to write, and the title check catches the per-project docs earlier releases wrote; it adds no record type, and the doc still shows on its first-use day page like any doc. Not chosen: a new record type (more code and site handling for one file), or a dated name (a second dated file could not be told from a successor).
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- --text=This repo's two feedback docs hold 24 entries (22 pm-harness, 2
  pm-quality) on 2026-10-10 13:10 UTC, and 0.3.0 keeps appending to them until
  the release ships: the pm-harness doc gained one entry (13:04 UTC) while
  this sprint ran. So the merge (.5.2) is a script rerun at apply time, not a
  frozen file.

- --text=Against a copy of the real records store with the real work store's
  items (621 items), the new checks fail the store before the merge
  (docs/2026-10-07-pm-harness-feedback: a repo keeps one pm feedback doc …)
  and pass after it: 164 pages, docs/pm-feedback renders 24 entries, 24 tagged
  with their project; entry bodies equal the old ones (24 = 24) and headings
  are in time order.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
