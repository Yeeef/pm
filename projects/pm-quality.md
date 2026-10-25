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

::: decision {source=owner date=2026-10-16}
pm --help hides the machinery commands (hook, prime, push, service run, sync, export, day summarize, autopilot) behind pm --help --all; they stay dispatched and documented. This revises pm-harness 18 "help output stays as it is".
The 2026-10-16 command audit found 13 machinery leaves typed 0-5 times each beside about 50 agent commands; the tree has grown from about 30 nodes when pm-harness 18 ruled to 81.
:::

::: decision {source=owner date=2026-10-16}
prime.md keeps full procedures for the commands that carry most use and names the rest in one line pointing at --help; inlined refusal lists move to --help. This tunes the 2026-10-07 deliberate-redundancy decision; it does not reverse it.
The audit measured prime.md at 3,218 words with its third hook chunk at 76% of the cap; the top 20 commands carry 91% of 3,768 invocations, and agents read refusals rather than --help (5 of 252 failures were followed by --help).
:::

::: decision {source=owner date=2026-10-17}
pm need close ID --reason closes every need kind that sets no rule; decision close, action done and need dismiss are removed, each refused with a message naming need close. decision add --need stays the path that records a rule. This revises pm-harness 13, which put each close verb under its noun.
Four verbs served one act: decision close ran 1 to 3 times and need dismiss 9 times in 11 days of transcripts, and a close under the wrong noun is refused.
Answers `pm-d2k5.39.1.2`.
:::

::: decision {source=owner date=2026-10-17}
Every pm command names its scope with --sprint, --task and --project only: decision need and action need drop --parent, decision add drops --level. Removed flags are refused with a message naming the new flag, with no alias. This revises pm-quality 21 for scope flags.
A scope was named five ways, a measured source of agent confusion; one vocabulary lets a grammar test hold new commands to it.
Answers `pm-d2k5.39.1.1`.
:::

::: decision {source=owner date=2026-10-25}
Agents raise what needs fixing in the repo as sprint proposals (pm sprint propose), the one mechanism for it; there is no repo feedback doc; pm feedback stays for pm itself
One mechanism keeps one triage place; a proposal carries a frame the owner can accept, where feedback prose waits for someone to turn it into a sprint; pm feedback is about a tool whose code is not in the repo
:::

::: decision {source=owner date=2026-10-25}
A sprint proposal needs no owner accept: it is an open sprint marked proposed, grouped in one collapsed Proposed section on the site, and becomes ordinary work when someone starts it
The owner wants proposals collected where they can browse them, not a triage queue to answer; an accept step adds a site write path and a wait for little gain
:::

## Design pages

> Where is the detail?

None yet.

## Outcome

> Written when the project closes: what was achieved against the goal, what
> was learned, what was retired, and links to the sprint delivery reports.

Not closed yet.
