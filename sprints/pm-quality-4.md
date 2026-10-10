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

done: `pm sprint move <sprint> --to <project>` moves an open sprint whole in one command, keeping its id, and a rerun finishes a move cut short (PR #24, pending merge).

- Design on the work-store page (Ids, Moving a sprint), with the alternatives not chosen; rows on the pm-cli and pm-go pages.
- The work store's write runs through the child-id compare-and-swap (`pm_move_sprint(?)`). It sets the parent, mints the new project's next number and adds a move note.
- One records commit then renames the record and adds a decision to both projects.
- A moved sprint's `number` now merges three-way.
- `pm show` names a sprint by its number.
- The old record path and the old site page lead to the moved record.
- `pm sprint open` never reuses a number that was moved away.
- `--help`, the `prime.md` procedure line, a CHANGELOG entry under Added, and CLAUDE.md.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **A sprint with two tasks, a decision and a finding moves to another project: met.** `tests/test_pm.py::test_sprint_move_takes_the_sprint_whole_and_a_rerun_finishes_a_move_cut_short` moves sprint demo-1, which holds:
  - a closed task;
  - a task that sess-a holds;
  - a decision need;
  - a sprint decision;
  - a finding.
- **`pm show --project` lists it under the new project only: met.** The same test checks that `pm show --project site` lists "Sprint 2: First" and that `pm show --project demo` does not.
- **Its tasks keep their holders: met.** The task's holder is still sess-a, and its parent is unchanged.
- **The old id resolves to the new place: met.** The id is kept, and `pm show repo-demo.1` shows the parent repo-site and the move note. The old record path also resolves: `pm show --record records/sprints/demo-1.md` gives the moved record.
- **A move interrupted after its first write leaves `pm show` consistent: met.**
  - The fault point is a read-only `sprints/` directory, so the records step fails after the work-store write.
  - The command fails and names the rerun.
  - The records files are unchanged.
  - `pm show`, `pm show --project` (both projects), `pm show <id>` and `pm check` are consistent.
  - The same command run again writes only the records step.
- **The other clone: met.** `TestMoveSprintAcrossClonesKeepsTheOtherClonesWrites` races the move's push with the other clone opening a sprint in the target project, adding a task under the sprint, and commenting on and renaming the sprint.
  - The move takes number 2 (the other clone's new sprint has 1).
  - The task lands in the moved sprint.
  - Both clones converge.
- **`make test` and CI pass: met.**
  - `make test`: 114 passed, 40 skipped.
  - `make test-go`: every package ok, race tests included.
  - The integration tests matching `-k 'show or sprint or record_link'`: 12 passed.
  - PR #24's CI is green: light, integration, build-and-test on darwin-arm64 and linux-amd64, changelog.
- **Open:** a rerun on a clone whose records branch has not synced can write the records step a second time (see Findings).
