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

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
