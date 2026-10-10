---
type: sprint
title: Every pm release carries a structured release note, enforced by CI
bead: yeeef-agents-9va.118
---

## Goal

> What should be true when this sprint ends, and why now?

Every pm release on GitHub carries a release note that says what shipped and changed in that version, split into Breaking changes (with upgrade steps), Added, Changed, Deprecated, Removed, Fixed and Security, following Keep a Changelog. CI enforces it, so a release with no proper note cannot be published. Why now: the owner judged the 0.3.0 note, one install line, far below the bar.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `CHANGELOG.md` in Yeeef/pm (Keep a Changelog 1.1.0, SemVer), with a checker that validates its format and extracts a version's section.
- The release workflow refuses a tag whose version has no valid section and publishes that section as the release body.
- A PR check that requires an `[Unreleased]` entry when shipped code changes (escape hatch: the `no-changelog` label).
- Backfilled entries for the Go releases 0.2.0 through 0.3.0, and their GitHub release bodies rewritten from them.
- AGENTS.md "Releasing pm" updated.

**Out:**
- Python-era releases (below 0.2.0).
- Generating notes automatically from commit messages.

## Done when

> What evidence will show the goal is met?

- The checker's tests pass, covering a valid changelog, a missing version section, an unknown category and an empty section.
- A release-candidate tag on the PR's branch with no changelog section fails the `version` job; with a section, it publishes that section as the body.
- The pm-v0.3.0 release page on GitHub shows the categorized note.

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

done: every pm release's GitHub note is now its CHANGELOG.md section, with an upgrade guide and categorized changes, and CI refuses a PR or a tag without one (Yeeef/pm PR #8, merged as 392308a).

Merged as 392308a (PR #8).

- `CHANGELOG.md` (Keep a Changelog 1.1.0) with entries for 0.2.0 through 0.3.0; each release opens with a required, numbered `### Upgrade guide`.
- `release/changelog.py` (`check`, `notes`, `pr`), with 12 tests.
- `pm-changelog.yml`: the PR check, with the `no-changelog` label to skip it. `pm-release.yml`: a tag fails without a section, and the section becomes the release body. `pm-release-notes.yml`: re-renders an old release's body.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Checker tests: met. `pytest tests/test_changelog.py` gives 12 passed; `make test` gives 147 passed, 138 skipped.
- rc tag fails without a section and publishes the section with one: met for the publish path. `pm-v0.3.1-rc.1` on 19697ff released in run https://github.com/Yeeef/pm/actions/runs/38019977165, and its pre-release body is the `[Unreleased]` section with its Upgrade guide. The no-section path is covered by unit tests only; no failing tag was pushed.
- pm-v0.3.0 release page shows the categorized note: met. Four `pm-release-notes.yml` runs succeeded, and `gh release view` shows Upgrade guide and the categories on 0.2.0, 0.2.1, 0.2.2 and 0.3.0.
