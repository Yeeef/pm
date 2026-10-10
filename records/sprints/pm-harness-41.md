---
type: sprint
title: A merged review tells the agent to pull main
bead: yeeef-agents-9va.47
---

## Goal

> What should be true when this sprint ends, and why now?

When a PR review's wait wakes the agent, its next step includes fast-forwarding the clone's main checkout, so the merged change takes effect there (hooks, rules and the ~/.claude links read the main checkout) without the owner asking.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the `next:` line `pm reply wait` prints for a review whose PR merged to main, and the one for an owner reply on a review card: both name `git -C <main checkout> pull --ff-only origin main`; the RULES.md closing step and tests.

**Out:** pm pulling by itself (the agent runs it and sees a refusal when the checkout has local work or is on another branch); pulling other worktrees.

## Done when

> What evidence will show the goal is met?

- A merged review's wake text ends with the pull command for this clone's main checkout; an owner reply on a review card says to run it once the PR is on main; tests check both.
- RULES.md's sprint-closing step says to pull the main checkout after the merge.
- `make test` passes.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
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

Done: a merged review's wake tells the agent to fast-forward the clone's main checkout.

Merged as eb434b6 (PR #45).

- PR #45: the pull command in `pm reply wait`'s next step for a merged review and for a reply on a review card; RULES.md's closing step; tests.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Merged review's wake ends with the pull; a review-card reply names it; tests check both: met.** `test_review_wait_ends_when_its_pr_merges_to_main` checks the exact text ending `then update the main checkout: git -C <main checkout> pull --ff-only origin main`; `test_review_wait_without_gh_ends_on_a_reply_only` checks the reply's `; once its PR is on main, update the main checkout: …` suffix; the plain-action reply test still checks its unchanged text. On this clone the path is ~/Desktop/workspace/yeeef-agents.
- **RULES.md's closing step says to pull the main checkout: met.** "fast-forward the main checkout with the command the wake names (`git -C <main checkout> pull --ff-only origin main`)".
- **`make test` passes: met.** 374 passed, 7 skipped.
