---
type: sprint
title: Close a PR review whose PR is in another repo
bead: pm-d2k5.42
---

## Goal

> What should be true when this sprint ends, and why now?

A pm repo can raise, close and stamp a PR review whose PR is in another GitHub repo, verified through gh, because the owner runs a pm project that manages several other repos and today the review cannot be closed (restamp, sprint close --merged and the merge watcher all check the sha against this repo's main).

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** telling another repo's PR from this repo's; checking `merged as <sha>` for another repo's PR against gh's merge commit and that repo's default branch (need close, restamp, sprint close --merged --pr); the raise check of the PR's base against that repo's default branch; the service's merge watcher for such a PR; the sprint Outcome naming the PR as OWNER/REPO#N; skipping this repo's design gate for another repo's PR; tests and help texts.
**Out:** design pages covering another repo's files; non-GitHub hosts for PRs.

## Done when

> What evidence will show the goal is met?

- A harness test raises a review for another repo's PR, closes it with `merged as <its merge sha>`, restamps a review first closed another way, and closes the sprint, all exit 0; a wrong sha is refused naming gh's merge commit.
- The service stores the merge of another repo's PR (a test).
- `make test`, `make test-go` and the PR's CI pass.

## Design pages

> Where is the detail?

- [Sprint lifecycle](../design/sprint-lifecycle.md): Close, a review of another repo's PR
- [Reply delivery](../design/reply-delivery.md): Merges of reviewed PRs

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-25}
The design page pm-cli stays as it is for this sprint's code changes.
The change adds another repo's PR to the review checks; this page's area is unchanged
:::

::: decision {source=agent date=2026-10-25}
The design page pm-go stays as it is for this sprint's code changes.
The change adds another repo's PR to the review checks; this page's area is unchanged
:::

::: decision {source=agent date=2026-10-25}
The design page session-claims stays as it is for this sprint's code changes.
The change adds another repo's PR to the review checks; this page's area is unchanged
:::

::: decision {source=agent date=2026-10-25}
The design page views-and-site stays as it is for this sprint's code changes.
The change adds another repo's PR to the review checks; this page's area is unchanged
:::

::: decision {source=agent date=2026-10-25}
The design page site-replies stays as it is for this sprint's code changes.
The change adds another repo's PR to the review checks; this page's area is unchanged
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Reproduced in a scratch harness (repo A, a review of o/other#7 whose merge
  commit is not in A): closing the open review with "merged as <B sha>" passed
  unchecked; a review first closed another way could not be restamped ("merged
  as abcdef0 is no commit of this repo, origin/main included"); sprint close
  --merged --pr refused the same way; a B PR touching paths A's covers match
  was refused by A's design gate; the service watcher never stored B's merge
  (review.merged still None after 5 s, set within 5 s for a same-repo
  control).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
