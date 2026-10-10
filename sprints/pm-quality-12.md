---
type: sprint
title: Changelog entries as one file per change, so parallel PRs never conflict
bead: pm-d2k5.12
---

## Goal

> What should be true when this sprint ends, and why now?

Two PRs that each add a changelog entry never conflict with each other, so a batch of parallel sprint PRs merges without a rebase round per merge. On 2026-10-10 eight pm-quality PRs were open at once; each merge left every other one conflicting in `CHANGELOG.md`'s `[Unreleased]` section, and each rebase cost a CI run of about 10 minutes.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- An entry is its own file under a directory such as `changelog.d/` (kind: Added, Changed, Fixed, Breaking, Upgrade step), as towncrier and changesets do; check prior art and pick the simplest form.
- `release/changelog.py` assembles the entries into `## [X]` at release time (the release PR), checks each entry file, and the PR check (`pm-changelog.yml`) accepts an entry file where it now wants an `[Unreleased]` edit.
- CLAUDE.md's "Releasing pm" and the changelog preamble describe the new flow; existing `[Unreleased]` entries move to files.

**Out:**
- Changing what an entry says or the release notes' layout.

## Done when

> What evidence will show the goal is met?

- A test: two branches that each add an entry, merged one after the other, give no conflict, and `changelog.py` renders both under the release.
- `release/changelog.py check` and `test_changelog.py` pass; the PR's CI passes.

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
