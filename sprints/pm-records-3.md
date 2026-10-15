---
type: sprint
title: Rename a design page or a doc
bead: pm-c72a.3
---

## Goal

> What should be true when this sprint ends, and why now?

An agent can rename a design page or a doc, its slug, its title or both, in one command that keeps every record's links to it working. Today a slug is fixed at `pm design new` / `pm doc new`: renaming one by hand leaves `Design pages` entries, prose links and design waivers pointing at a page that no longer exists, and `pm check` does not notice.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm design rename <slug> [--to NEW] [--title "…"]`: moves `records/design/<slug>.md`, sets its title, and rewrites every `../design/<slug>.md` link and design waiver line in the records, in one commit.
- `pm doc rename <path|slug> [--to NEW] [--title "…"]`: the same for a doc; the date prefix stays.
- Refusals: an unknown or existing target, the feedback doc, a page named by an open PR review need.
- `--help` texts, golden files, harness tests, changelog entry, design page update.

**Out:**
- Redirects from old URLs.
- Rewriting review needs in the work store (an open one refuses the rename instead).

## Done when

> What evidence will show the goal is met?

- `make test` and the PR's CI pass, with harness tests that rename a design page and a doc and assert the moved file, the new title, the rewritten links and waivers, and each refusal.
- A live run in a scratch clone renames a design page that a sprint's Design pages section links, and `pm check` passes after it.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
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

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
