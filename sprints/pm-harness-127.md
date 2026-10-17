---
type: sprint
title: Upgrade pm to 0.15.4
bead: yeeef-agents-9va.137
---

## Goal

> What should be true when this sprint ends, and why now?

This repo pins pm 0.15.4, so its sessions run the release that lists a sprint's design pages on `pm sprint open` and on a session's first `pm task claim`.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm upgrade --to 0.15.4` in a worktree, its changes committed, one PR.

**Out:**
- Any code change.

## Done when

> What evidence will show the goal is met?

- The PR that moves `.pm/config.toml` to `version = "0.15.4"` is on main, with green CI.

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

done: this repo pins pm 0.15.4 once PR #110 merges.

- `.pm/config.toml` moves `version` from 0.15.2 to 0.15.4 (`pm upgrade --to 0.15.4`); no other file changes.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- The PR that moves the pin is on main, with green CI: CI met (PR #110: light, integration, build-vet-test, race, work, lint, check, changelog all pass); on main once merged.
