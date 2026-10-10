---
type: sprint
title: pm CLI v2
bead: yeeef-agents-9va.7
---

## Goal

> What should be true when this sprint ends, and why now?

Single-step `bd` actions that carry harness rules go through `pm` too, so the
rules hold without relying on agents remembering them.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** `pm task add` (tasks always inside an open sprint), `pm task close`
(names the commit; trunk gates once sprint 4 defines them), `pm task move`
(records the scope change as a sprint decision).

**Out:** commands already in v1 (sprint 6).

## Done when

> What evidence will show the goal is met?

- `pm task add --sprint ID` creates a task only inside an open sprint, and
  refuses otherwise (`yeeef-agents-9va.7.1`).
- `pm task close` closes a task with a reason naming the commit when there is
  one, and warns when there is none; trunk gates wait for sprint 4
  (`yeeef-agents-9va.7.2`).
- `pm task move` moves a task to another open sprint and records a sprint
  decision in the sprint it leaves, in one step (`yeeef-agents-9va.7.3`).
- Each refusal has a test; `make test` and `make render` pass; RULES.md and
  the skill point agents at these commands instead of plain `bd`.

## Design pages

> Where is the detail?

- [pm CLI](../design/pm-cli.md): the single-step commands (`pm task add`, `pm task close`, `pm task move`)

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- pm task add needs no own project check: loading the records already refuses
  a sprint record whose epic is not under a project record ('no project record
  found'), so the refusal comes from validation.

- pm task close judges whether HEAD holds a task's work by commit time against
  the task's started_at (created_at if never claimed); a task closed with no
  newer commit gets no hash and a warning, and --commit names an older one.
  Used for real to close 9va.7.2 (ad858eb) and 9va.7.3 (eab9375).

- All three commands also ran against a real bd in a throwaway repo (bd init,
  pm project open, two pm sprint open): task add created demo-99q.1.1 and
  refused the project epic; task move reparented it to demo-99q.2 (bd show:
  parent demo-99q.2) and wrote the decision; task close named HEAD and refused
  the epic and the closed task; pm render passed.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: pm task add, pm task close and pm task move exist, each refusal is
tested, and RULES.md and the skill send agents to them instead of plain bd.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- `pm task add --sprint ID` creates a task only inside an open sprint: met.
  It refuses an unknown or non-sprint id, a closed sprint, a non-epic, a
  sprint outside every project record, and an empty title
  (`test_task_add_refuses*`); commit f338fae. Ran against a real bd in a
  throwaway repo, creating demo-99q.1.1 and refusing the project epic.
- `pm task close` names the commit when there is one and warns when there is
  none: met. HEAD when committed after the task started, or `--commit`; a
  warning with no hash otherwise, and on a dirty tree; refuses epics, needs,
  unknown and closed tasks (`test_task_close_*`); commit ad858eb. Used for
  real to close 9va.7.2 and 9va.7.3. Trunk gates wait for sprint 4.
- `pm task move` moves a task to another open sprint and records a sprint
  decision in the sprint it leaves, in one step: met. Refusals and the undo
  hint are tested (`test_task_move_*`); commits eab9375 and 08e32fb (leaving a
  closed sprint is allowed). Ran against a real bd: demo-99q.1.1 reparented
  to demo-99q.2 with the decision written.
- Each refusal has a test; `make test` (122 passed) and `make render` pass;
  RULES.md, SKILL.md and the design page point agents at the commands:
  met, commits 21cda0f and 08e32fb.
