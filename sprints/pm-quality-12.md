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

- A head's statusCheckRollup can hold one check twice: PR #27's has 'pm
  changelog / changelog' CANCELLED (cancel-in-progress when a label changed)
  then SUCCESS, on the same head. merge_ready.py judges each check by its
  latest run, and treats a check with any run not completed as pending.

- Live: make merge-ready PR=32 right after the push of b05fcd9 listed 5 checks
  queued or in progress and 'no check of workflow pm go … on its head yet':
  the pm go workflow had not registered a check yet, so a rollup of only the
  checks present would have under-counted.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: each unreleased change is its own `changelog.d/<slug>.md`, which no other PR touches, and the merging agent merges only a PR whose CI ran on a head that contains main (PR #32, pending merge).

- `changelog.d/<slug>.md` holds the `###` categories a change touches, checked by the release sections' rules; `CHANGELOG.md` holds released sections only, and `changelog.py check` refuses an `## [Unreleased]` there.
- `changelog.py release X --summary …` writes `## [X]` from the entries with its link and deletes them; `notes X-rc.N` assembles the same section; `pr BASE` wants an entry file; released notes unchanged.
- `make merge-ready PR=N` (`release/merge_ready.py`): refuses unless the PR is open, its head contains `origin/main`, every check on that head passed (by its latest run), and each every-PR workflow has its checks there; prints `gh pr merge N --squash --match-head-commit <sha>`. No repo setting changed.
- The `[Unreleased]` entries from #30 moved to `changelog.d/help-names-every-command.md`; AGENTS.md (Releasing pm, Tests, Layout) and the changelog preamble describe the new flow.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Two branches that each add an entry, merged one after the other, give no conflict and render both under the release: met. `test_two_branches_that_each_add_an_entry_merge_without_conflict_and_both_reach_the_release` cuts both from one base, merges them in turn with `git merge --no-ff`, then `changelog.py release 0.3.0` renders both entries' bullets under `[0.3.0]`, and `notes 0.3.0` prints them.
- `release/changelog.py check` and `test_changelog.py` pass; the PR's CI passes: met. `changelog.py check` printed `CHANGELOG.md: ok, 6 releases; changelog.d/: ok, 1 entries`. `uv run pytest tests/test_changelog.py tests/test_merge_ready.py`: 24 passed. `make test`: 136 passed. PR #32's CI on head b05fcd9: all 11 checks passed. `make merge-ready PR=32` said ready (head b05fcd9 contains origin/main cee2d7d). Released notes 0.2.0 to 0.5.0 and 0.5.0-rc.1 are byte-identical to main's script's output. Merge sha: PR #32, pending merge.
