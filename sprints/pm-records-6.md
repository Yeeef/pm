---
type: sprint
title: Drafts across clones, and this repo on the new record layer
bead: pm-c72a.6
---

## Goal

> What should be true when this sprint ends, and why now?

A draft made in one clone shows on every other clone's site, a sprint closed from any clone publishes it, and this repo runs the release that carries the whole record layer. Today drafts live only in their clone: a sprint closed from clone B leaves clone A's drafts tagged to a closed sprint, and an owner's site shows only the drafts of the machine it runs on. This is step 5 of [record-drafts](../design/record-drafts.md), then the project's last step: move this repo's pin.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Per-clone draft refs `refs/pm/drafts/<clone>/<name>`: a clone id made once by `pm init`; the service pushes each draft commit with `--force-with-lease` against the sha it last pushed, flags a lease failure, and fetches others' refs; other clones list them as committed drafts with no session.
- `pm sprint close` publishes the sprint's pages from fetched draft refs too, and deletes a remote ref once all its pages are published.
- A PR moving this repo's pin with `pm upgrade --to` the release that carries steps 1 to 5, merged through `make merge-ready`; a live check on this repo after the upgrade.

**Out:**
- Live (uncommitted) drafts from other machines: only committed drafts travel.

## Done when

> What evidence will show the goal is met?

- A second clone lists the first clone's draft as a version after one follow and one sync each, and a close from the second clone publishes it (a test with two clones of one bare origin).
- A lease failure is flagged, never forced over (a test).
- CI green on the sprint's PR, merged through `make merge-ready`.
- This repo pins the new release; after the upgrade, in a fresh worktree here, `records/` is a workspace, a record edit shows as that worktree's version on the site, and `pm commit` publishes it (live check, numbers in Findings).

## Design pages

> Where is the detail?

- [Record drafts and publishing](../design/record-drafts.md): Draft refs, Publish at sprint close, Increment 5
- [Decision need layout](../design/decision-need-layout.md): covers (the `needform` pattern, on main only after pm-quality sprint 38 merges, removed for the pin move)
- [Shared records store](../design/records-store.md): Push's drafts step; pm where's clone line
- [Sprint lifecycle](../design/sprint-lifecycle.md): the close publishes other clones' drafts, fetched first
- [pm CLI](../design/pm-cli.md): pm sprint close fetches the other clones' draft refs
- [pm in Go](../design/pm-go.md): internal/sync pushes and fetches the draft refs
- [pm as an installable product](../design/pm-product.md): pm init gives the clone its id

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-16}
The design page agent-sync stays as it is for this sprint's code changes.
The sync gains a drafts step; this page keeps the agent profile and leaves how records and drafts push to records-store and record-drafts, which this sprint updated
:::

::: decision {source=agent date=2026-10-16}
The design page reply-delivery stays as it is for this sprint's code changes.
run.go changes only the sync steps (the drafts step); reply delivery is untouched
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Live check of per-clone draft refs (this branch's .go/pm 0.9.0-dev, commit
  bfa2158): two scratch clones A and B of one scratch bare origin under
  /tmp/pmlc9x776xys, each with HOME, CODEX_HOME and CLAUDE_CONFIG_DIR of its
  own and the harness's fake_sched as supervisor (each service served its
  site). pm init gave each clone an id (A 40ccd2085839, B its own; pm where:
  'clone     <id>  its drafts push to origin refs/pm/drafts/<id>/'). In A's
  worktree wa, pm commit drafted records/design/parser.md for the sprint (0.06
  s); A's pm push: 'drafts ok: pushed 1 and deleted 0 draft refs' (0.20 s),
  origin then held refs/pm/drafts/40ccd2085839/wa. B, cloned after, ran pm
  init and one pm push (0.07 s, 'fetched the other clones''); B's site listed
  the version 'clone 40ccd2085839, worktree wa, sprint r-yqb2.1, no session'
  0.25 s later, its published page without A's edit and
  drafts/40ccd2085839~wa/design/parser.html with it. pm sprint close from B
  (0.10 s): 'published design/parser.md' plus a note naming A's draft, exactly
  1 records commit with 'Published: design/parser.md r-yqb2.1', A's text in
  it, on B's site 0.25 s later; A's remote ref left untouched by B. After B's
  pm push and A's pm push: A 'drafts ok: pushed 0 and deleted 1 draft refs',
  origin held no draft ref. Found on the way, not this change's: A's records
  step then failed, as both clones had committed days/2026-10-16.summary.json
  and the rebase conflicted on it (the records-store page's open question on
  two machines).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
