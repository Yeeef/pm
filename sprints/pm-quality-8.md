---
type: sprint
title: pm task close and move cover dropped, held, foreign-commit and orphan tasks
bead: pm-d2k5.8
---

## Goal

> What should be true when this sprint ends, and why now?

Closing or moving a task records what happened, with no stand-in commits and no silent override of another session.

Today:
- `pm task close` records every close as done and asks for a commit, so a dropped task is closed with an unrelated records commit (formal-methods 2026-10-07 20:34; pm 2026-10-10 03:59).
- It warns about a dirty tree even when `--commit` names the work (pm 2026-10-10 03:59).
- It resolves `--commit` only in this repo (pm 2026-10-09 19:23).
- It closes a task that another live session holds, which `pm task claim` refuses (pm 2026-10-10 04:00).
- `pm task move` refuses a task directly under a project. The site lists such tasks under "Not in a sprint", and the rules say to bring them into a sprint (pm 2026-10-09 16:06).

Raised by the pm feedback triage of 2026-10-10 across pm, formal-methods, ai-safety and yeeef-agents.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A not-done close: `pm task close ID --dropped` with a required reason and no commit, shown as dropped in `pm show` and on the site. Choose before any code whether this is a new resolution, which every clone's store check (`work/check.go:85`) must accept so an upgrade step follows, or the existing `dismissed`. Record the choice as a sprint decision.
- Warn about the dirty tree only when `--commit` is not given.
- `--commit OWNER/REPO@SHA` and a PR URL, resolved with `gh`. One that does not resolve is refused.
- `pm task close` refuses a task that another live session holds, with the same liveness check and wording as claim, and names `pm task release`.
- `pm task move` from directly under a project into one of that project's open sprints, recording the scope added as a decision in the sprint it joins.
- `--help` texts, prime.md's close and move lines, and a CHANGELOG entry.

**Out:**
- Moving a whole project to another repo (0.4.0 `pm init --import` and pm-quality sprint 4).
- A backlog object.

## Done when

> What evidence will show the goal is met?

- Harness tests:
  - a `--dropped` close writes its resolution and reason, warns nothing and needs no commit;
  - a close with `--commit` in a dirty tree prints no warning;
  - `--commit Yeeef/pm@<sha>`, answered by `fake_gh`, puts that commit in the reason, and an unknown one is refused with `repo.unchanged()`;
  - a close of a task that another live session holds is refused with `repo.unchanged()`;
  - a project-level task moves into a sprint, and that sprint's record gains the decision.
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
A dropped task closes with the existing dismissed resolution: pm task close ID --dropped --reason "…" writes it with no commit, and pm show --sprint, the site and the day page show a dismissed task as dropped. No new resolution.
The work store syncs between clones through refs/pm/work and every clone checks each item (work/check.go accepts done, answered, no-decision, dismissed), so a new resolution would make every clone still on an older pm refuse the synced store until it upgrades: a cross-clone migration. dismissed already means closed without delivering, which is what a dropped task is; on a task only the bd import (a Dismissed close reason) wrote it before, since undoCreate closes only projects and sprints, so the label dropped misrepresents nothing.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- The Done-when harness tests pass: 8 tests (-k 'task_close or
  task_move or task_claim', 5 new) in 2.4 s; make test 117 passed, 40 skipped
  in 6.4 s; make test-go ok (13 packages); the integration tests that render
  pages or the day summary (-k 'serve_shows_each_change or day_summarize or
  every_store_command') 4 passed.

- make test-go run from the main checkout fails
  TestOnlyTheHostOpensTheStoreAndOnlyTheServiceStartsIt
  (internal/work/access_test.go): its walk from the repo root skips .git, .go,
  testdata and .venv but not .claude, so the 12 agent worktrees under
  .claude/worktrees add 24 NewHost callers. From a worktree, and in CI, it
  passes. Not fixed here (out of scope); skipping .claude in the walk would
  fix it.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: `pm task close` drops a task with a reason and no commit, names work in another repo or a PR, warns about the tree only when HEAD stands in, and refuses a live session's task; `pm task move` takes a project-level task into a sprint (PR #21, pending merge).

- `--dropped`: resolution `dismissed` (sprint decision: no new resolution, no cross-clone upgrade), shown as dropped in `pm show --sprint`, the task graph and legend, the day page and the day summary; left out of both numbers of "n of m tasks done".
- `--commit OWNER/REPO@SHA` (`gh api`) and a PR URL (`gh pr view`); this repo's refs first; an unresolvable one, or a PR closed unmerged, refused.
- `pm task close` refuses a task another live session holds, with claim's wording, naming `pm task release`.
- `pm task move` from directly under a project into one of its open sprints; the decision goes in the sprint it joins.
- `--help`, `prime.md`, CHANGELOG `[Unreleased]`, the day-summary prompt and the design pages work-layer, work-store and sprint-lifecycle.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- A `--dropped` close writes its resolution and reason, warns nothing, needs no commit: met, `test_task_close_dropped_records_why_and_needs_no_commit` (resolution `dismissed`, reason kept, stderr empty in a dirty tree, `pm show --sprint` prints `dropped`, `pm show --project` counts 1/2).
- A close with `--commit` in a dirty tree prints no warning: met, `test_task_close_with_commit_warns_nothing_about_a_dirty_tree` (and without `--commit` it still warns).
- `--commit Yeeef/pm@<sha>` answered by `fake_gh` puts that commit in the reason; an unknown one is refused with `repo.unchanged()`: met, `test_task_close_resolves_another_repos_commit_or_a_pr_with_gh` (also a merged, an open and a closed PR, and a local branch named like `OWNER/REPO@SHA`).
- A close of a task another live session holds is refused with `repo.unchanged()`: met, `test_task_close_refuses_a_task_another_live_session_holds`.
- A project-level task moves into a sprint and that sprint's record gains the decision: met, `test_task_move_takes_a_task_from_directly_under_a_project_into_its_sprint`.
- `make test` and the PR's CI pass: `make test` 117 passed, 40 skipped; `make test-go` ok; CI on PR #21: see the PR's checks (the first push passed light, integration, changelog, guard and linux build-and-test).
