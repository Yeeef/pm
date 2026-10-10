---
type: sprint
title: Dogfood on yeeef-agents
bead: yeeef-agents-9va.1
---

## Goal

> What should be true when this sprint ends, and why now?

The harness's work layer (Beads) and record format are proven on real work
in this repo, so the CLI and views are built from what real records needed.
Design: [Project management harness](../design/pm-harness.md).

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**

- Beads set up and used for this sprint's own work.
- Repo guidance condensed for agents.
- Project, sprint and day records written by hand.
- Records rendered to a local site.

**Out:**

- The `pm` CLI.
- Adopting the harness in poker-ai.

## Done when

> What evidence will show the goal is met?

- `make docs` renders every record with live Beads status.
- The open design questions (`yeeef-agents-9va.1.6`) are decided.

## Design pages

> Where is the detail?

- [Work layer: Beads](../design/work-layer.md): how our concepts map to Beads
- [Record layer](../design/record-layer.md): the project, sprint and day record templates
- [Views and the site](../design/views-and-site.md): the views rendered from records

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-01}
Dogfood on yeeef-agents, not poker-ai, because this repo is small and a
mistake here costs nothing, while poker-ai has live work running.
:::

::: decision {source=owner date=2026-10-01}
No `pm` CLI in this sprint. Records are written by hand, so the CLI is built
for what real records needed rather than what we guessed.
:::

::: decision {source=agent date=2026-10-02}
Built a renderer and `make docs` instead of rendering one daily page by
hand, because a hand-rendered page would have been thrown away. Task
`yeeef-agents-9va.1.4` was renamed to match; the scope change is recorded
here rather than only in the task title.
:::


## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Writing the records by hand surfaced three format questions: attribute
  values with spaces need quotes (`until="sprint close"`); the sprint's
  findings had no home; `date` on a decision duplicates git history.
- Beads ids use the repo name as prefix (`yeeef-agents-9va`), not `bd-a1b2`.
- Epics show in `bd ready`; `bd ready --exclude-type=epic` removes them.
- `bd ready --assignee=<owner>` gives the "needs you" list directly.(superseded by the `human` label)
- Beads has a `decision` issue type (alias `adr`). Not yet checked whether
  it can replace `::: decision` blocks.
- `bd init` does more than its README says: commits on its own, takes over
  git hooks, writes Codex and Cursor files, and edits `CLAUDE.md`.
- Keep `bd remember` for small operational facts; `bd prime` loads all of
  it every session. Decisions go in records.
- Until this record existed, the sprint frame lived only as prose in the
  epic's description.
- Agents claim tasks under the owner's git identity, so "needs you" (open
  tasks with an assignee) also lists work an agent is doing. Agents need
  their own Beads actor, or needs need a label. Resolved: needs are issues
  labelled `human` (`bd human list`), which Beads already supports.
- A raw HTML block in Markdown ends at the first blank line, so a mock-up
  with a `<pre>` inside must have no blank lines (or use `&#8203;`).
- Moving a task to another sprint (`bd update --parent`) keeps its id, so
  `yeeef-agents-9va.1.7` now lives in sprint 6. Ids show where a task was
  created, not where it is.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: Beads and the record format held up on real work in this repo, and
`make docs` renders every record with live Beads status.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **`make docs` renders every record with live Beads status:** met.
  `make render` builds 11 pages (root, project, 6 sprints, 2 days, design
  page); status, needs and progress come from `bd list --all --json`.
- **The open design questions are decided:** met. `yeeef-agents-9va.1.6`
  was answered and closed on 2026-10-03; the answers are project decisions.

The one open task, `yeeef-agents-9va.1.7`, moved to sprint 6.
