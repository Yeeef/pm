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

- 2026-10-10: PRs #18 and #23 each passed the rules chunk cap alone, but main
  after both was 10,018 of 10,000 characters, and main's CHANGELOG had '###
  Added' twice. Each PR's CI ran against its own base, not the merged result,
  so main went red. Two PRs that touch disjoint lines can still break a
  whole-file invariant together.

- Prior art: towncrier (newsfragments/<id>.<type>, type in the name,
  'towncrier build' assembles and deletes), changesets (.changeset/<name>.md,
  YAML front matter per package, 'changeset version' consumes), scriv
  (changelog.d/<name>.md holding '### Category' sections, 'scriv collect').
  Chosen: scriv's form, changelog.d/<slug>.md holding '###' categories, since
  changelog.py's category parser checks it unchanged and one PR may touch
  several categories (#30's entry has Added and Fixed); no front matter.
  GitHub's merge queue is unavailable on a user-owned repo.

- Every release 0.2.0 to 0.5.0 opens its upgrade guide with the same two steps
  (install pm-v<X>/install.sh; pm upgrade --to X), so 'changelog.py release X'
  writes those two and numbers the entries' own steps after them; released
  notes for 0.2.0..0.5.0 and 0.5.0-rc.1 are byte-identical between main's
  changelog.py and the new one.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
