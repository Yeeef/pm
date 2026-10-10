---
type: sprint
title: Sprints close on merge
bead: yeeef-agents-9va.23
---

## Goal

> What should be true when this sprint ends, and why now?

A sprint closes only when its PR is merged to main, and the owner reads a draft delivery report on the sprint page while reviewing the PR. This applies the owner's decision answering `yeeef-agents-9va.22.5`: done should mean delivered to main, and a report written before review can describe a state main never got.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the draft marker on a delivery report and the banner it shows; `pm sprint close` checking the sprint's PRs with `gh` and naming the merge commits; review actions filed under the sprint so they block its close; RULES.md and the skill (draft report before the review request, close after the merge). The work was done as task `yeeef-agents-9va.4.2` under the former sprint 4 and moved here when sprint 4 became the agent-sdlc project.
**Out:** the rest of the implementation cycle (work trunks, review depth, commits and traceability), which is the agent-sdlc project.

## Done when

> What evidence will show the goal is met?

- A delivery report marked "Draft until its PR merges." (or naming the PRs) after the Outcome's verdict shows a "Draft — closes when PR #N merges" banner on the sprint page, and the review card links the draft report; `make render` fails on a closed sprint whose report is still a draft.
- `pm sprint close` asks nothing of GitHub: it refuses a draft report or any open task, and the open review is such a task, closed by the agent only once the PR is on main (owner decision on `yeeef-agents-9va.23.2`).
- Review actions (`pm action need --pr`) are filed under the sprint and block its close until `pm action done`.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-04}
A draft delivery report is marked by a paragraph 'Draft until its PR merges.' (or naming the PRs: 'Draft until PR #14 and PR #15 merge.') after the Outcome's verdict sentence; the sprint page shows a 'Draft — closes when PR #N merges' banner and the review card links the draft report; render fails on a closed sprint whose report is still a draft.
The verdict stays the Outcome's first paragraph, so pm versions without drafts (main and other branches) still accept the record; a 'Draft:' prefix before the verdict broke them (store commit 2762bb2, reverted).
:::

::: decision {source=agent date=2026-10-04}
The PR that delivers a sprint is recorded by its review action (pm action need --pr, metadata.review.pr and .sprints), which goes under the first sprint named and requires every named sprint open with a draft report; pm sprint close checks every review naming the sprint, dismissed ones included, with gh pr view, refuses until each is merged (skipping a PR closed unmerged whose review was dismissed) (and refuses a sprint with no review unless voided, or with gh missing), and names the merge commits beside the records commit.
The review already stores the PR and its sprints, so a separate field would duplicate it; a review placed under the sprint blocks its close until pm action done, which undoes the project-epic placement 9va.22.2 tried.
:::

::: decision {source=agent date=2026-10-04}
The agent finalises the report by hand (removing the 'Draft until … merges.' paragraph) and commits it before pm sprint close; the close refuses a draft rather than stripping the marker itself, and an open sprint with a final report stays valid.
Keeping the close a read-and-close step matches pm sprint close today and leaves the finalised wording to the agent; forbidding a final report on an open sprint would make the finalising commit fail the render check.
:::

::: decision {source=agent date=2026-10-04}
Moved yeeef-agents-9va.4.2 here from sprint 4 (yeeef-agents-9va.4), with the three decisions above copied unchanged, when sprint 4 became the agent-sdlc project and was deleted.
The close-on-merge work belongs to pm-harness, not to the new project; pm task move refuses a closed task, so the move was bd update --parent.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- pm commit cannot commit a deletion: with a record already removed, git add
  -A on its path fails, so deleting sprint 4's record needed a plain git
  commit in the store.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: a sprint's delivery report is a draft until its PR merges, reviews
sit under the sprint and block its close, and `pm sprint close` stays a
local check with no GitHub call.

- Draft line "Draft until … merges." after the verdict; the sprint page shows
  a "Draft — closes when PR #N merges" banner, and review cards link it.
- `make render` fails on a closed sprint whose report is still a draft.
- Reviews (`pm action need --pr`) go under the sprint again.
- `pm sprint close`: no GitHub check (owner decision on 9va.23.2); the agent
  closes the review only once the PR is on main.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Draft marker, banner, review-card link and the closed-draft render check:
  met (a167daf); `test_draft_report_shows_a_banner_naming_the_pr`,
  `test_render_refuses_a_closed_sprint_with_a_draft_report`.
- `pm sprint close` with no GitHub check, refusing a draft report or an open
  task: met (f559cfd); `test_sprint_close_refuses_draft_report`,
  `test_sprint_closes_after_its_pr_merges`.
- Reviews filed under the sprint and blocking its close: met (a167daf);
  `test_action_need_pr_card_links_pr_sprints_design_pages_and_focus`.
