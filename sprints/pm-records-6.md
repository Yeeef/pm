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
- [Agent lifecycle](../design/agent-lifecycle.md): the Stop hook on the own workspace, its conflict markers and stranded pages
- [Views and the site](../design/views-and-site.md): Versions lists other clones' drafts

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

::: decision {source=agent date=2026-10-16}
The design page pm-in-codex stays as it is for this sprint's code changes.
pieces.go changed only its package doc comment to name the clone id; the Codex hooks, rules and roots pm init writes are unchanged
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

- Live check on this repo after the pin move to 0.15.1: a worktree made as Claude Code makes one (`git worktree add --no-checkout`, then `git reset --hard`) got its records workspace on `drafts/livecheck-records` from `pm init --session-start` in 0.14 s; this finding was written by hand in that workspace.

- Live check on this repo, pm 0.15.1, real service on :8001: the hand edit in
  the workspace left /sprints/pm-records-6.html without it (200, 0 matches)
  and listed the version /drafts/livecheck-records/sprints/pm-records-6.html
  (200, 1 match) with its diff (200); pm commit from the worktree took 0.31 s
  and landed as records 0e7f6e5; 2 s later the published page held the line (1
  match) and the workspace was clean. After pm init on the main checkout the
  service reported the new drafts step overdue since 2026-10-10 until its
  first run; the first pm push cleared it (drafts ok: pushed 0, deleted 0).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: drafts travel between clones and a close from any clone publishes them, and this repo runs pm 0.15.2, the release that carries the whole record layer.

- Per-clone draft refs `refs/pm/drafts/<clone>/<name>`, pushed with a lease and never forced; other clones list them as versions; `pm sprint close` fetches and publishes them (PR #105, merged as cd8d0e2).
- A store only behind its remote fast-forwards, refusing any uncommitted or ignored path it would overwrite; a page tagged for a closed sprint is kept under a ref, never lost to `git worktree remove` (PR #105).
- A sync step a newer pm adds starts its overdue clock with the service that runs it (PR #107, merged as d3719e9).
- The pin moved 0.11.0 to 0.15.1 (PR #106, merged as 6d374b4), then to 0.15.2 (PR #108, merged as 41d85a7).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- A second clone lists the first clone's draft after one follow and one sync each, and a close from it publishes the draft: met. Go tests with two clones of one bare origin (`internal/store/drafts_refs_test.go`); live check with two scratch clones: A's draft pushed in 0.20 s, B's site listed it 0.25 s after B's sync, B's close made 1 records commit, A's next push deleted its ref (Findings).
- A lease failure is flagged, never forced over: met. Go test; the other writer's sha stays on the remote.
- CI green on the sprint's PRs, merged through `make merge-ready`: met. PRs #105, #106, #107 and #108, each merged with the command `make merge-ready` printed; three fresh-context reviews of #105, every correctness finding fixed with a test.
- This repo pins the new release, and in a fresh worktree here `records/` is a workspace, an edit shows as that worktree's version, and `pm commit` publishes it: met. Pin 0.15.2, `pm check` renders all 245 pages; live check on this repo with the real service: workspace on `drafts/livecheck-records` in 0.14 s, the published page unchanged and the version and diff 200 before the commit, `pm commit` 0.31 s, published 2 s later (Findings).
