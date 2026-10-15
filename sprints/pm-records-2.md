---
type: sprint
title: Published view and page versions on the site
bead: pm-c72a.2
---

## Goal

> What should be true when this sprint ends, and why now?

The owner reads each record's published version on the site, and every draft of it beside it, so live edits are visible and labelled instead of mixed into the page. Today the site renders the store's working tree, so a half-made edit shows as if published and one malformed record turns every page into a 500. This is the first increment of the [record-drafts](../design/record-drafts.md) design (Model A, the owner's choice), split so the site side ships before the per-worktree workspaces: the version machinery takes drafts from a source that the workspaces plug into next.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- The published view renders the records at the `records` ref, not the working tree.
- Drafts as a source the site lists: for now the shared store's uncommitted edits, as one draft named for the store.
- The version bar on a page with drafts; each version's full render at `/drafts/<name>/<record path>.html`; its diff against published, section by section, and draft against draft.
- A malformed draft fails only its own version.
- A live check: whether a `claude -w` session can write a git worktree nested inside its own worktree, and whether Codex can; the answer fixes where workspaces live, recorded in the design page.

**Out:**
- Per-worktree workspaces, follow, journal publish (the next sprint).
- The Stop hook, publish at merge, draft refs.

## Done when

> What evidence will show the goal is met?

- In a scratch clone, a hand edit to a design page leaves its published page unchanged and adds a version; the version renders at its URL and its diff shows exactly the edited lines (a test, and a live check on the scratch service).
- A malformed draft serves an error only at its own version URL; every published page answers 200 (a test).
- The site's integration tests pass in CI on the sprint's PR.
- The `claude -w` and Codex live check's commands and results are in the sprint's Findings, and the design page's Open questions and Workspaces say where workspaces live.

## Design pages

> Where is the detail?

- [Record drafts and publishing](../design/record-drafts.md): Published view and versions; Workspaces (where they live)

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-15}
The design page increment 1 is split: this sprint ships the published view and versions over the shared store; workspaces, follow and journal publish follow in the next sprint.
The site side is independent of the workspaces and gives the owner the version view first; the claude -w live check that fixes where workspaces live runs here, before workspaces are built.
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
