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

::: decision {source=owner date=2026-10-12}
pm-quality PRs merge as soon as make merge-ready passes, with no owner review; the agent merges and closes the sprint with --merged
Owner, 2026-10-12: quality-improvement PRs need no owner approval; CI and merge-ready are the gate
:::

::: decision {source=owner date=2026-10-14}
The Dolt GC double-close bug (wg.Done before EndOutstandingVisitCall in gc_safepoint_controller.go) is not filed upstream; pm keeps its workaround in Host.GC, whose start-up check of Dolt unexported fields forces a re-check at the next Dolt upgrade. The issue text stays in pm-quality sprint 24 report.
Owner, in chat 2026-10-14: keep the workaround only.
:::

::: decision {source=owner date=2026-10-14}
A PR runs on macOS only the tests whose behaviour can differ by OS, plus the darwin build; the full Go suite runs on macOS on every push to main
Owner, 2026-10-14: most tests generalize across OSes; the macOS runner (3 cores, 5 concurrent jobs) is the PR critical path; a post-merge full run still catches what the subset misses. Revises the 2026-10-11 keep-macOS-on-every-PR decision
:::

## Design pages

> Where is the detail?

None yet.

## Outcome

> Written when the project closes: what was achieved against the goal, what
> was learned, what was retired, and links to the sprint delivery reports.

Not closed yet.
