---
type: sprint
title: "pm sprint move: move a sprint to another project in one command"
bead: pm-d2k5.4
---

## Goal

> What should be true when this sprint ends, and why now?

One command moves an open sprint, with its frame, tasks, decisions and findings, to another project. Today a move is six hand steps (pm-site sprint 1 was moved that way on 2026-10-10), and a failure between them leaves a half-moved sprint that only a hand check finds.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pm sprint move <sprint> --to <project>` in Go pm, with a reason body recorded as a decision in both projects' trails.
- What happens to ids and record paths (a sprint id is a child of its project epic, its record named `<project>-<n>.md`): renumber with a pointer from the old id, or keep the id and reparent. Chosen in a design page section before code.
- The move is all or nothing: a failure part way leaves the sprint where it was, or a rerun completes it.
- The `--help` text, the move procedure line in both `prime.md` files, and a test.

**Out:**
- Moving a closed sprint.
- Moving tasks between sprints, which `pm task move` already does.
- Merging two sprints.

## Done when

> What evidence will show the goal is met?

A test moves a sprint with two tasks, a decision and a finding to another project: `pm show --project` lists it under the new project only, its tasks keep their holders, the old id resolves to the new place, and a move interrupted after its first write leaves `pm show` consistent. `make test` and the PR's CI pass.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- --text=Design: keep the id and renumber the record (work-store page, Moving
  a sprint). The id is what review targets, blockers, tasks and chat name, so
  keeping it makes every old reference resolve with no pointer to follow.
  Renumbering with a new id would strand a task that another clone adds under
  the old id before it syncs. The old project's number is held by a move note
  (a comment, author pm), not by a schema change, which would force every
  clone to upgrade in lockstep as 0.3.0's did.

- --text=Review caught a sync break before push: merge.go refused any two
  different sprint numbers. Before this change no number changed; now that a
  move changes it, a clone that wrote the sprint's row while another moved it
  would fail every later sync. number now merges three-way.
  TestMoveSprintAcrossClonesKeepsTheOtherClonesWrites covers it: the other
  clone comments on and renames the sprint during the move's push race.

- --text=Open edge, not fixed: two clones can both write the records step. A
  rerun runs the step when the sprint is in the target project and its record
  still has the old name. On a clone whose records branch has not synced,
  another clone's finished move looks the same, so the rerun writes the rename
  and both decisions again, and the decisions are duplicated once the branches
  sync.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
