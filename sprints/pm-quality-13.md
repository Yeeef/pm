---
type: sprint
title: Review follow-ups from the 2026-10-10 sprint wave
bead: pm-d2k5.13
---

## Goal

> What should be true when this sprint ends, and why now?

The low-severity defects that fresh-context reviews and sprint agents found in the 2026-10-10 wave of pm-quality PRs are fixed, each with the check that catches it, so none of them stays known and unfixed.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `records.Texts` and Load: a record that vanished mid-read is skipped (PR #15), which bypasses Load's retry under the records lock; restore a consistent read (retry under the lock when anything was skipped) without bringing back the error page.
- A dangling `.md` symlink in the records store is an error again for `pm check` and `pm commit` (it became silently ignored in PR #15).
- `pm uninstall` racing a parallel session start that restarts the service between its unsynced check and the unit's removal; `service.Uninstall` disabling a systemd unit only when it is active, which leaves an enabled, crashed unit's `default.target.wants` link (found in sprint 11's review; both predate it).
- `pm sprint move`'s records step can run twice across clones when a clone whose records branch has not synced reruns a move another clone finished, duplicating the rename and decisions (sprint 4).

**Out:**
- New features; anything a running sprint already covers.

## Done when

> What evidence will show the goal is met?

- Each item has a test that fails before its fix and passes after, and `make test`, `make test-go` and the PR's CI pass.

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
