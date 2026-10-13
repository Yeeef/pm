---
type: sprint
title: "Pin moves across many releases: every crossed release's steps shown, old pins pruned"
bead: yeeef-agents-9va.127
---

## Goal

> What should be true when this sprint ends, and why now?

Releases now come on every merge, so a repo's pin move often crosses several releases, and each one leaves a binary behind. A pin move must show the owner every upgrade step of every release it crosses, so no step is missed. A machine must not keep every release ever pinned: about 113 MB each, never pruned.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm upgrade --to X` prints the upgrade guide steps of each release after the current pin up to X, read from the release notes (the CHANGELOG.md of X), grouped by release, before it rewrites anything.
- Pruning launcher pins (`<data dir>/pm/pins/<X>`) that no repo on this machine pins, with the kept sha256 rule intact: a pruned pin's sha256 record stays, so a later download is still checked.
- When pruning runs (pm upgrade, pm doctor, or a command), and what it reports.
- Tests, `--help`, and the design pages of the areas changed.

**Out:**
- Moving pins automatically.

## Done when

> What evidence will show the goal is met?

- A pin move across three releases prints each release's steps in order; a harness test with a local release server shows it.
- After a pin move, the old pin's binary is gone when no repo on the machine pins it, and its sha256 record stays; a test shows a re-download of a pruned pin is still checked against the record.
- Every existing design page whose area this sprint changes is brought to the new state and listed in Design pages.

## Design pages

> Where is the detail?

- [How a repo runs the right pm version](../design/pm-versioning.md): Pin moves, the crossed releases' guides and pruning (covers `internal/launch/**`).
- [pm in Go](../design/pm-go.md): the pins cache and `pm upgrade` rows (covers `internal/launch/**`).
- [pm as an installable product](../design/pm-product.md): the machine's pins, moving the pin, `pm upgrade` and `pm doctor` (covers `internal/install/**`, `internal/service/unit*.go`).
- [When and how pm releases](../design/release-policy.md): Pin moves, and its two open questions, now answered.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-13}
pm upgrade --to X takes the crossed releases upgrade guides from the release notes pm X embeds (CHANGELOG.md and changelog.d/ via assets.go), not from the network; X own guide is its section, else its entry files assembled as release/changelog.py does.
pm upgrade --to X always runs pm X, which carries every earlier release notes, so no download, token or new release asset is needed and nothing can be unreachable; under the release-on-merge flow the build precedes the release commit, so X section can only come from its entries.
:::

::: decision {source=agent date=2026-10-13}
The machine registry of clones is the pm service units (one per clone, written by pm init) plus the clone the command runs in; each clone worktrees are read for their pins. No new registry file.
Every clone set up by any pm since 0.2.0 already has a unit, so clones on old pins are seen too; a registry that only new pm versions write would miss them and prune pins they still use.
:::

::: decision {source=agent date=2026-10-13}
pm upgrade prunes on every run (a move, or --to the pin); pm doctor names prunable pins without failing; pm clean does not prune, unlike the release-policy default.
pm upgrade is the one command that changes pins and runs as the new pin, so pruning sits with it; the main checkout moves only after the PR merges, so doctor names what remains and pm upgrade --to the pin prunes it; keeping pm clean to worktrees avoids a second command with delete powers over the pins.
:::

::: decision {source=agent date=2026-10-13}
The design page pm-cli stays as it is for this sprint's code changes.
This sprint changes only the pm upgrade and pm doctor help texts in commands.go; pm-cli describes the command tree, parsing and code structure, none of which changed.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Pins in use on this machine (4 clones with units, every worktree read,
  2026-10-13): 0.1.0, 0.3.0, 0.5.0, 0.6.0, 0.7.1, 0.8.0; only 0.4.0 (118 MB)
  is prunable. Old worktrees, not repos, hold most old pins, so pm clean
  (removing merged worktrees) is what frees them for pm upgrade to prune.

- PR #63 builds a release at the merge commit, before its release commit
  writes X's section into CHANGELOG.md, so pm X's embedded notes lack its own
  section; Guides falls back to X's entry files, assembled as changelog.py
  does (Go test holds the renumbering; the harness test compares with
  changelog.py on the real notes).

- go:embed fails the build when a pattern matches an empty directory: one
  pattern names CHANGELOG.md and changelog.d/ so a checkout without
  changelog.d/ builds, but a local changelog.d/ left empty (changelog.py
  release deletes the files, not the dir) fails make go-build until it is
  removed. git never checks out an empty dir, so CI is not affected.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: `pm upgrade --to X` prints the upgrade guide of every release it crosses before it writes, and prunes the launcher pins no checkout on the machine pins, keeping each one's sha256; the PR waits on review.

- Guides from the release notes pm X embeds (`assets.go` `Notes`, `internal/launch/guide.go`): no network; X's own guide from its section, else its entry files as `release/changelog.py` assembles them; a crossed release without a guide fails before any write.
- Pruning (`internal/install/pins.go`, `service.Clones`): the clones are this one and each with a pm service unit; every worktree's pin is kept; anything unreadable prunes none. `pm doctor` names prunable pins without failing.
- PR: https://github.com/Yeeef/pm/pull/65

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **A pin move across three releases prints each release's steps in order: met.** `test_upgrade_prints_the_upgrade_guide_of_every_release_it_crosses_before_it_writes` (`tests/test_launch.py`) moves a repo from 0.7.0 to this build: it prints 0.7.1, 0.8.0, 0.8.1, 0.8.2 and the build's own guide, in that order and before "moved the pin", each equal to what `release/changelog.py` reads from the real notes. The steps come from the notes pm X embeds, not from a server (sprint decision), so this test needs no release server; the pruning test below uses one. `internal/launch/guide_test.go` covers the version edges (pre-releases, rc to rc, a downgrade, an untagged build, a missing guide failing), and `tests/test_changelog.py` holds its assembled guide to `changelog.py assemble` on one shared fixture.
- **After a pin move the old pin's binary is gone when no repo pins it, its sha256 stays, and a re-download is checked: met.** `test_upgrade_prunes_the_pins_no_checkout_pins_and_a_pruned_pin_downloads_checked` (local release server through `PM_RELEASE_URL`): the repo moves off 0.2.0, `pm upgrade` prunes `pins/0.2.0/pm` and keeps `pins/0.2.0/sha256` unchanged, keeps 0.3.0, which another clone with a unit pins; a tampered 0.2.0 then fails with "changed since this machine first downloaded it" and writes nothing; the original downloads again. `internal/install/pins_test.go`: worktree pins (one whose path holds a newline) and unit clones kept; three unreadable cases prune none. Live, read-only, on this machine: only 0.4.0 (118 MB) is prunable (Findings).
- **Every design page whose area changed is at the new state and listed: met.** pm-versioning (new Pin moves subsection), pm-go, pm-product and release-policy (its two open questions answered) are edited and listed in Design pages; pm-cli is waived (only help texts changed).
- Checks: `make lint` 0 issues; `make test-go` pass; `make test` 89 passed; `tests/test_launch.py`, `test_config.py`, `test_changelog.py` and every `-k "upgrade or doctor"` test pass; CI on PR #65 (head 09e2ec7): all 14 checks pass. A fresh-context review found no correctness issue; its minor findings are fixed in the PR.
