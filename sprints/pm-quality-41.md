---
type: sprint
title: Reopen a closed project or sprint
bead: pm-d2k5.41
---

## Goal

> What should be true when this sprint ends, and why now?

A closed project or sprint can be reopened with `pm project reopen` and `pm sprint reopen`, each recording its reason as a decision in the reopened record, so an owner's "reopen it" keeps the history in one place instead of a continuation project. The owner asked for it directly ("please allow project, sprint to be reopened"); a poker session had to open a continuation project for want of it (that repo's pm-feedback, 2026-10-19).

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A work-store reopen of a closed item (status open, close fields cleared), in the Store interface and its implementations.
- `pm project reopen <name>` and `pm sprint reopen <id>`: a two-line-or-more reason body recorded as a decision; the close-time section (Outcome, Delivery report) back to its not-closed text, so a later close needs a fresh one.
- Refusals: not closed; a sprint whose project is closed (naming `pm project reopen`).
- `--help`, the command tree, prime.md, help goldens, a changelog.d entry.
- Black-box tests: the round trip and each refusal.

**Out:**
- Reopening tasks or needs.
- A release, or moving any repo's pin.

## Done when

> What evidence will show the goal is met?

- `make test` and `make test-go` pass, and the PR's CI is green.
- A black-box test closes a project and a sprint, reopens each, sees status open and the decision in the record, and closes each again after a fresh Outcome / Delivery report.
- Black-box tests see each refusal's text.

## Design pages

> Where is the detail?

- [Sprint lifecycle](../design/sprint-lifecycle.md): Reopen; the stage diagram's reopen
- [pm CLI](../design/pm-cli.md): Naming, the reopen verb; Command families
- [Work store: pm's own replacement for Beads](../design/work-store.md): Inside pm, Reopen
- [Record layer](../design/record-layer.md): a reopen puts the close-time section back
- [pm in Go](../design/pm-go.md): the served package; internal/item holds pm export's lines
- [Views and the site](../design/views-and-site.md): covers internal/cli/served

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-19}
The design page agent-lifecycle stays as it is for this sprint's code changes.
prime.md gains one How line naming the reopen commands; the agent lifecycle is unchanged
:::

::: decision {source=agent date=2026-10-19}
The design page decision-need-layout stays as it is for this sprint's code changes.
decision.go changes only a comment that said the work store has no reopen
:::

::: decision {source=agent date=2026-10-19}
The design page pm-product stays as it is for this sprint's code changes.
prime.md gains one How line naming the reopen commands; setup and the product are unchanged
:::

::: decision {source=agent date=2026-10-19}
The design page record-drafts stays as it is for this sprint's code changes.
commit_workspace_test.go only calls the served site under its new package name
:::

::: decision {source=agent date=2026-10-19}
The design page reply-delivery stays as it is for this sprint's code changes.
decision.go changes only a comment that said the work store has no reopen
:::

::: decision {source=agent date=2026-10-19}
The design page session-claims stays as it is for this sprint's code changes.
dolt.go gains Store.Reopen, which leaves holders as they are; move.go gains pm sprint reopen, which claims nothing
:::

::: decision {source=agent date=2026-10-19}
The design page site-replies stays as it is for this sprint's code changes.
serve.go only builds the site from its new package, internal/cli/served; the reply forms and access are unchanged
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- The size caps left no room for the feature: internal/cli held 7359 of its
  7365-line ratchet and internal/work exactly 5000 of 5000. Two
  behavior-preserving moves made room (the served site to internal/cli/served,
  169 lines; pm export's writer to internal/item, 130 lines); after the
  feature cli holds 7307 (the new ratchet) and work 4873. A first try moved
  the bd import's record reader to internal/records, but that made
  internal/work's tests import records, so a records change would select the
  slow work tests (tests/test_ci.py caught it); it was dropped.

- The fresh-context review found that a reopen on one clone merged against a
  reopen-and-close on the other produced an open item with close fields
  (status, close group and resolution merged apart), which the item check
  refuses; mergeClose now lets closed win when one side closed anew and clears
  close fields on an open result (MergeAReopenOnOneSideStandsWhole).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: a closed project or sprint reopens with `pm project reopen` / `pm sprint reopen`, recording the reason as a decision and putting its close-time section back for a fresh close.

- `Store.Reopen`, and a merge rule under which closed wins against a reopen when the other side closed anew
- `pm project reopen <name>`, `pm sprint reopen <id>`: reason body, `--confirmed`, refusals for an open item and for a sprint of a closed project
- room under the size caps: the served site to `internal/cli/served`, `pm export`'s writer to `internal/item`
- help, golden, prime.md, changelog entry, six design pages

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- `make test` and `make test-go` pass, and the PR's CI is green: met locally (`make test` 156 passed, 169 skipped live; `make test-go` 19 packages ok, work 33 tests in 8 shards, race 10); CI green on [PR #116](https://github.com/Yeeef/pm/pull/116) (10 checks pass, 4 skipped as not selected; lint-go first failed on the moved lines' unchecked writes and a test's complexity, fixed).
- A black-box test closes, reopens and closes again a project and a sprint: met, `test_a_closed_project_and_sprint_reopen_with_the_reason_and_close_again_on_a_fresh_report` (status open, decision in the record, records compared whole, a fresh report needed for the second close).
- Black-box tests see each refusal's text: met in the same test (open project, open sprint, sprint of a closed project, one-line reason), and `test_every_store_command_fails_hard_with_the_service_stopped` covers both commands.
