---
type: sprint
title: Rename a sprint, edit a need, stamp an agent's merge
bead: pm-d2k5.9
---

## Goal

> What should be true when this sprint ends, and why now?

Four write-path gaps close:
- A sprint whose goal changed can take a matching title. Today `task edit` refuses a sprint id and `sprint` has only open and close (pm 2026-10-10 02:14).
- An open need's body can be edited (formal-methods 2026-10-10 03:23).
- A sprint whose PR an agent merged closes with its "Merged as" stamp. Today the stamp comes only from a closed review need (formal-methods 2026-10-07 20:45). The owner decided that agents merge pm-quality's own PRs (project decision, 2026-10-10), so every pm-quality sprint now closes without the stamp.
- `pm commit` accepts a path relative to the store, such as `sprints/x.md` (pm-quality doc 2026-10-10 04:31).

Raised by the pm feedback triage of 2026-10-10 across pm, formal-methods, ai-safety and yeeef-agents.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm sprint edit ID --title "…"` with a reason body. It changes the work-store title and the record's `title:` header in one write, and records the reason as a sprint decision.
- `pm need edit ID --text-file -` for an open action or decision need. It refuses once the need holds a reply.
- `pm sprint close ID --merged SHA [--pr URL]`. The SHA must be on the remote's main. The stamp is the one a review close writes. It is refused when the sprint holds a review need.
- `pm commit` resolves `sprints/…`, `docs/…` and other store-relative paths as it resolves `records/…`.
- `--help`, prime.md's close line, and a CHANGELOG entry.

**Out:**
- An explicit order for needs. `pm dep add` already orders them, and S1 makes it visible.
- Editing closed items.

## Done when

> What evidence will show the goal is met?

- Harness tests:
  - `pm sprint edit` changes both titles and adds the decision;
  - `pm need edit` changes the body of an open need and refuses one that holds a reply;
  - `pm sprint close --merged <sha on main>` stamps "Merged as <sha>", a SHA not on main is refused with `repo.unchanged()`, and a sprint that holds a review need refuses `--merged`;
  - `pm commit -m … sprints/x.md` commits `records/sprints/x.md`.
- `make test` and the PR's CI pass.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-10}
`pm need edit` takes an action's body with `--text`/`--text-file`, and a decision need's parts with the flags `pm decision need` takes (`--question`, `--fact`, `--option`, `--cost`, `--default`), not a free `--text-file` body.
A decision need has one checked layout (question, facts, options with costs, default, 25-word sentences); a free body would bypass those checks, so the edit rebuilds it through the same code.
:::

::: decision {source=agent date=2026-10-10}
The work store itself refuses a new description for a need that is closed or holds a reply (`Dolt.Edit`, inside the write transaction), besides the CLI refusal.
A reply that lands between the CLI check and the write would otherwise let the body change under an answer; the store check closes that window on one clone.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- The store already kept a sprint title's 'Sprint <n>: ' prefix in step with
  its number (work/merge.go on a concurrent edit, work/move.go on a move), so
  pm sprint edit only had to write the prefix it found; the record header
  holds the bare title, as pm sprint open writes it.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
