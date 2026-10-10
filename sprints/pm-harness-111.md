---
type: sprint
title: Release pm 0.3.1
bead: yeeef-agents-9va.121
---

## Goal

> What should be true when this sprint ends, and why now?

pm 0.3.1 is published, carrying PR #7's one request rule (prime.md, the owner-request judge and its reprompt), so a repo can move its pin to it. Why now: the owner asked for a patch release on 2026-10-10, after PR #7 merged.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A PR to Yeeef/pm that turns CHANGELOG.md's [Unreleased] into the [0.3.1] section, with PR #7's entry, a summary and the upgrade guide.
- The tag pm-v0.3.1 on that PR's merge commit, and the release workflow's run.

**Out:**
- Moving any repo's pin to 0.3.1 (a separate PR per repo).

## Done when

> What evidence will show the goal is met?

- gh release view pm-v0.3.1 shows a release (not a pre-release) with both tarballs, SHA256SUMS and install.sh, and the 0.3.1 changelog section as its notes.
- install.sh from the release installs a binary whose pm version prints 0.3.1, in a scratch bin dir.

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
