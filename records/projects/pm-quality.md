---
type: project
title: "pm quality: code, architecture, tests and feedback"
bead: pm-d2k5
---

## Goal

> Why do we do it? What is it? What outcome do we expect?

pm's code, architecture and tests stay at the standard of a great open-source project, and what agents report as friction gets fixed: this project holds the sprints that improve pm's code quality, architecture design and test quality, and the sprints that address entries in pm's feedback doc. Features and integrations live in their own projects.

## Progress

> Where are we now, and what's next? Generated from the work store and the
> sprint records when the page is rendered. Do not write here.

## Decisions

> What constrains every future sprint? Sprint-only choices live in the sprint
> record.

::: decision {source=owner date=2026-10-10}
A fix for a defect that can recur ships with the mechanism that catches its class (a type, an assertion, a test, a lint rule, a CI check, a hook, a script); a prose rule or a hand fix is the fallback for what no mechanism can check.
Owner, 2026-10-10: prefer structural fixes; also added to the global agent rules (Yeeef/yeeef-agents PR #104).
:::

::: decision {source=owner date=2026-10-10}
The coordinating agent merges pm-quality PRs once CI is green and a fresh-context review finds no correctness issue, and cuts releases; no PR review need is raised.
Owner, 2026-10-10: you have agency to merge PRs and take actions; autopilot mode.
:::

## Design pages

> Where is the detail?

None yet.

## Outcome

> Written when the project closes: what was achieved against the goal, what
> was learned, what was retired, and links to the sprint delivery reports.

Not closed yet.
