---
type: sprint
title: Request cards link their sprint
bead: yeeef-agents-9va.35
---

## Goal

> What should be true when this sprint ends, and why now?

A decision or action card shows the sprint it belongs to, not only the project, so the owner sees the context of each request at a glance; most requests are raised under a sprint or a sprint's task.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the cards under "Decisions await you" and "Actions await you" on the overview and day pages, and `pm show`'s request lines: name and link the sprint (and the task, when the request sits under a task), found from the request's parent chain in Beads; requests under a project directly keep showing the project.

**Out:** changing where requests are filed.

## Done when

> What evidence will show the goal is met?

- A request under a sprint or a sprint's task shows that sprint as a link to its page, beside the project; a test covers a request under a sprint, under a task, and under a project.
- Checked on the real site.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
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

Done: each decision and action card names and links the sprint (and task)
its request belongs to, beside the project, and `pm show` names it too.

- Card line: "Project · Sprint · task", with project and sprint linked, on the
  overview and day pages.
- `pm show`: `(sprint 15)` or `(sprint 1, task .1.4)` after each request;
  `--json` carries `sprint` and `task` ids.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- A request under a sprint or a sprint's task shows that sprint as a link
  beside the project, with a test for sprint, task and project placement:
  met (d397c1d);
  `test_request_cards_and_show_lines_name_the_sprint_and_task_a_request_sits_under`.
- Checked on the real site: met. The overview's card for 9va.34.2 links
  "Sprint 29: Site pages never wait on writers"; the task form was checked
  only in the test, since no open request sits under a task.
