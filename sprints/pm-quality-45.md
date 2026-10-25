---
type: sprint
title: Agents propose sprints for what the repo needs
bead: pm-d2k5.45
---

## Goal

> What should be true when this sprint ends, and why now?

An agent that finds something in the repo worth fixing or improving, outside its task, records it as a sprint proposal: `pm sprint propose` writes a full sprint frame plus the evidence it saw, as an open sprint labelled proposal that nobody has started. The site collects proposals in one collapsed Proposed section; `pm show` lists none. A proposal becomes ordinary work when someone adds its first task, or is dropped with a reason. Why now: the owner wants agents to raise this work directly, and chose proposals as the one mechanism for it (pm-quality project decision); today such findings stay in chat or are lost.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm sprint propose <project> --title "…"`: the frame (`## Goal`, `## Scope`, `## Done when`) plus an `## Evidence` section; its output lists the project's proposals and open sprints, so an agent can drop its own as a duplicate of one.
- A proposal is an open sprint with the label `proposal` and no task: `pm show` (every level, `--json` too) and `pm task ready` leave it out; `pm show --sprint ID` and `pm show ID` show it by id. Adding its first task starts it; `pm sprint close --dropped --reason` drops it.
- The site shows proposals in one collapsed Proposed section.
- `prime.md`: propose when you can name a concrete fix and its check; start a proposal only when the owner asks. `pm feedback` stays for pm itself.
- A design page for the proposal lifecycle.

**Out:**
- Repo feedback docs (dropped by the project decision).
- Turning pm feedback entries into proposals.
- Asking for proposals at SessionEnd (sprint 25's hook).

## Done when

> What evidence will show the goal is met?

- In a test repo, `pm sprint propose` writes a proposal that `pm show` (text and `--json`) and `pm task ready` leave out and `pm show --sprint ID` shows; its first task starts it, after which `pm show` lists it; `--dropped` closes it with its reason; the harness tests pass.
- The site shows proposals in a collapsed Proposed section, checked live in a scratch clone.
- `prime.md` and `--help` name the rule; the golden and hook tests pass.
- The PR's CI is green.

## Design pages

> Where is the detail?

- [Sprint proposals](../design/sprint-proposals.md): the proposal lifecycle: the `proposal` label and derived state, the commands, what `pm show` and the site show, the tests.
- [pm CLI](../design/pm-cli.md): Naming gains `sprint propose` and `sprint close --dropped`.
- [Work store](../design/work-store.md): `labels` names the one label pm reads, `proposal`.
- [Views and the site](../design/views-and-site.md): the Proposed section on the overview and project page, the day page's proposal rows.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-25}
A proposal is an open sprint with the label proposal, not a new status; no schema migration
The design review showed a new status forces every clone to upgrade and breaks the merge rules (a merged row can fail Check on every clone), while the existing labels table merges row by row and already serves as a type (bug)
:::

::: decision {source=owner date=2026-10-25}
pm show lists no proposals at any level, nor in --json; only the site shows them, and pm sprint propose prints the project's proposals for its duplicate check; pm show --sprint ID still shows one by id
The owner wants pm show to stay the state of running work; proposals are for the owner to browse on the site
:::

::: decision {source=agent date=2026-10-25}
The frame drops the owner accept and dismiss: a proposal is an open sprint labelled proposal that its first task starts, pm show lists none, the site shows them in a collapsed Proposed section, and pm sprint propose prints the duplicate list in its output
Follows the owner decisions of 2026-10-25 (no accept; pm show lists no proposals) and the agent decision for a label over a new status
:::

::: decision {source=agent date=2026-10-25}
The design page agent-lifecycle stays as it is for this sprint's code changes.
only small helper or wording edits for proposals; the page states nothing they change
:::

::: decision {source=agent date=2026-10-25}
The design page pm-product stays as it is for this sprint's code changes.
only small helper or wording edits for proposals; the page states nothing they change
:::

::: decision {source=agent date=2026-10-25}
The design page record-layer stays as it is for this sprint's code changes.
only small helper or wording edits for proposals; the page states nothing they change
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Live check in a scratch origin and clone (dev build at 7223483, own service
  on :33479, HOME and CODEX_HOME in a temp dir): 5 of 5 steps pass. propose
  lists the project's open sprints and proposals; pm show, --project and
  --json name no proposal; pm task ready offers none; the site's collapsed
  Proposed (2) opens on click in headless Chromium and links to the sprint
  page with its evidence finding; a first task makes it an open sprint
  (Proposed (1)); close --dropped shows DROPPED with its reason; reopen puts
  it back in Proposed.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
