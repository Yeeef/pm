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

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
