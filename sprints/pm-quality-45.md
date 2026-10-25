---
type: sprint
title: Agents propose sprints for what the repo needs
bead: pm-d2k5.45
---

## Goal

> What should be true when this sprint ends, and why now?

An agent that finds something in the repo worth fixing or improving, outside its task, records it as a sprint proposal: `pm sprint propose` writes a full sprint frame plus the evidence it saw, in a proposed state that is not running work. The owner accepts a proposal, which makes it an ordinary open sprint, or dismisses it with a reason. Why now: the owner wants agents to raise this work directly, and chose proposals as the one mechanism for it (pm-quality project decision); today such findings stay in chat or are lost.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm sprint propose <project> --title "…"`: the frame (`## Goal`, `## Scope`, `## Done when`) plus an evidence line; it lists the project's open sprints and proposals first, so an agent can cite one instead of raising a duplicate.
- A proposed state in the work store: not claimable, not counted as open or running work in `pm show`, with its own short list there.
- The owner accepts or dismisses a proposal from the site; an agent the owner asked can run the same from the CLI.
- `prime.md`: propose when you can name a concrete fix and its check; a rough frame is fine, the owner edits it at accept. `pm feedback` stays for pm itself.
- A design page for the proposal lifecycle.

**Out:**
- Repo feedback docs (dropped by the project decision).
- Turning pm feedback entries into proposals.
- Asking for proposals at SessionEnd (sprint 25's hook).

## Done when

> What evidence will show the goal is met?

- In a test repo, `pm sprint propose` writes a proposal that `pm show` lists apart from open sprints and `pm task ready` does not offer; accept makes it an open sprint, dismiss closes it with its reason; the harness tests pass.
- The site shows the proposals with accept and dismiss, checked live in a scratch clone.
- `prime.md` and `--help` name the rule; the golden and hook tests pass.
- The PR's CI is green.

## Design pages

> Where is the detail?

- [Sprint proposals](../design/sprint-proposals.md): the proposal lifecycle: a sprint status, the commands, the site's verdict, the tests.

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
