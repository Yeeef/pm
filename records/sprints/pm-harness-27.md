---
type: sprint
title: Review requests always use the review form
bead: yeeef-agents-9va.32
---

## Goal

> What should be true when this sprint ends, and why now?

Every request to review or merge a PR uses the review template (`pm action need --pr URL --sprint ID --focus "…"`), so its card links the PR, sprints and design pages, says what to focus on, and its wait wakes on the merge. On 2026-10-05 another session raised "Merge PR #31" (yeeef-agents-9va.30.4) as a plain action, which pm accepted.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** `pm action need` (and `pm decision need`) refusing a request whose title or description names a GitHub PR URL or "PR #N" unless it is raised with `--pr`, with a refusal that shows the review form; RULES.md.

**Out:** checking requests already raised; the Stop hook's judgement.

## Done when

> What evidence will show the goal is met?

- `pm action need --title "Merge PR #31…"` without `--pr` is refused with the review form shown; with `--pr`, `--sprint` and `--focus` it is accepted; tests cover both and a request that only mentions a PR in passing is decided explicitly (refused or allowed, recorded as a sprint decision).
- RULES.md says a PR review or merge request is always raised with `--pr`.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-05}
A request that names a PR is refused without `--pr` only when it also asks to review, merge or approve it (those words anywhere in its title or description); a PR named in passing, such as the decision "Should we split PR #31?", is allowed.

Refusing every PR mention would block decisions about a PR with no way to raise them, since `pm decision need` has no `--pr`; the review/merge/approve wording is what makes a request a review. A request that only mentions a PR next to those words is refused too, and the refusal says to rephrase it.
:::

::: decision {source=owner date=2026-10-05}
The review-form check stays pattern-based (a PR link or "PR #N" plus review, merge or approve), with no Haiku judgement; it works the same in Claude Code and Codex.
The owner chose option A: instant and predictable, and its refusal tells the agent how to reword or use --pr.
Answers `yeeef-agents-9va.32.2`.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: pm refuses a PR review or merge request raised without `--pr` and
shows the review form, so every such request gets the review card and wakes
on the merge.

- The check is patterns (owner decision on 9va.32.2): a PR link or "PR #N"
  plus review, merge or approve; a PR named in passing is allowed.
- Known false refusal: "after PR #30 merged, run X"; the refusal says to
  reword.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- A PR merge request without `--pr` is refused with the review form, and
  accepted with `--pr`, `--sprint` and `--focus`; the in-passing case is
  decided: met (bb95a3d);
  `test_a_pr_review_or_merge_request_without_pr_is_refused_with_the_review_form`,
  `test_a_pr_merge_request_with_pr_sprint_and_focus_is_accepted`,
  `test_a_pr_named_in_passing_is_allowed`; in-passing mentions allowed (sprint
  decision).
- RULES.md says PR review and merge requests are always raised with `--pr`:
  met (bb95a3d).
