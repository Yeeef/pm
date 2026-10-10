---
type: sprint
title: Sprint close without a draft report
bead: yeeef-agents-9va.48
---

## Goal

> What should be true when this sprint ends, and why now?

Closing a sprint after its PR merges is one `pm sprint close`, with no hand-made draft state: the sprint page shows the sprint's open needs (its PR review included) where the draft banner was, and the close stamps the merge commit into the report and commits it.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** removing the `Draft until its PR merges.` paragraph from the harness (its parsing, `pm action need --pr`'s requirement, `pm sprint close`'s and `make render`'s refusals, the site's draft banner); `pm action need --pr` requiring a written Outcome and "Against Done when"; generated "Decisions await you" and "Actions await you" sections on each sprint page for that sprint's open needs; `pm sprint close` requiring the review closed as "merged as <sha>", appending "Merged as <sha>." to the Outcome, committing the record and closing the epic; RULES.md, SKILL.md and templates; tests. Existing closed sprints whose reports still name a draft paragraph or were finalised by hand render unchanged.

**Out:** `pm` checking GitHub itself (the review's close reason stays the merge evidence); rewriting old records.

## Done when

> What evidence will show the goal is met?

- No harness code, rule, template or test refers to a draft report; `make test` passes.
- A sprint page with an open PR review shows it under the sprint's "Actions await you" with its PR link and reply box, and no draft banner; checked on the live site.
- On a sprint with a written report and its review closed as merged, one `pm sprint close` appends "Merged as <sha>.", commits the record on the records branch and closes the epic; it refuses with an open task, an open review, or a review not closed as merged. Tested, and used to close this sprint.

## Design pages

> Where is the detail?

- [Sprint lifecycle](../design/sprint-lifecycle.md): the stages from open to close, without a draft report
- [Record layer](../design/record-layer.md): the sprint record's delivery report (under The sprint record)

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- make render fails on this server before reading any record: uv resolves
  render.py's requires-python >=3.10 to a managed 3.10, and
  harness/site.py:108 has a backslash inside an f-string expression (legal
  only from 3.12), a SyntaxError; pm.py (>=3.11) imports site.py too. The
  design pages rendered with --python 3.12 (75 pages). To fix in this sprint
  after the code task lands, since site.py is being edited.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: a sprint's report has no draft state; its page shows its own open needs and `pm sprint close` stamps the merge and commits it.

Merged as e63ca73 (PR #46).

- PR #46: draft paragraph removed; `pm action need --pr` requires a written report; sprint pages render their own "Decisions await you" / "Actions await you"; `pm sprint close` stamps "Merged as <sha> (PR #N).", commits and closes, skipping dismissed reviews; Python floor 3.12 so `make render` runs on the server.
- Design: new sub page sprint-lifecycle; record-layer, pm-cli, views-and-site and the main page updated.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **No harness code, rule, template or test refers to a draft report; `make test` passes: met.** `make test` 383 passed, 7 skipped. The "draft" words left in skills/ are a CSS class for the push banner, a JS variable for an unsent reply, and the tests that an old record holding the paragraph still renders.
- **A sprint page with an open PR review shows it under its "Actions await you", no draft banner, on the live site: met.** With review 9va.48.3 open, this sprint's page rendered by the PR's code showed its card (`need-yeeef-agents-9va.48.3`, "Pull request: …/pull/46") under Actions await you between Progress and Decisions, with no draft banner; `test_sprint_page_lists_its_own_requests_the_pr_review_included` covers it. After the merge, the live site on :8000 (restarted on main e63ca73) serves the page with both await sections, now empty as the review is closed, and 0 "Draft — closes" banners.
- **One `pm sprint close` stamps, commits and closes; refuses an open task, an open review, or a review not closed as merged: met.** Sprint 41 was closed with it on 2026-10-06 (stamp "Merged as eb434b6 (PR #45).", records commit dc5c303), and so is this sprint. Tests cover the stamp with two reviews (one under another sprint), no review, a dismissed review skipped, the stamp's position, and each refusal writing nothing.
