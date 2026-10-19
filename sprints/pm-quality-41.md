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
