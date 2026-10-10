---
type: sprint
title: pm show discloses project state level by level
bead: yeeef-agents-9va.66
---

## Goal

> What should be true when this sprint ends, and why now?

An agent sees only top-level project state at session start, and reads deeper levels of `pm show` (a project, a sprint, a record section) when its work needs them. Why now: `pm show` is 7,300 characters and grows with each open sprint, and at session start it shares Claude Code's 10,000-character hook limit with pm's rules, so its tail is cut.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** `pm show` levels: a top level (tasks held by live sessions, undelivered owner replies, open owner requests, a failed push, the site, one line per project), and deeper levels per project, sprint and record section, each naming the command for the next level; `pm prime` prints the top level only; help, rules text and tests.
**Out:** the site; new state that `pm show` does not have today; the pm service (sprint 45).

## Done when

> What evidence will show the goal is met?

- The top level is at most 1,500 characters in this repo today, and it keeps every held-task warning, undelivered reply, open owner request and push failure; tests show each.
- Every line that `pm show` prints today is reachable through some level, shown by a test that compares the union of the levels with today's output.
- `pm prime` total is under 7,000 characters in this repo, and nothing in it is cut.

## Design pages

> Where is the detail?

- [pm CLI](../design/pm-cli.md): what each level of `pm show` prints, with samples
- [Agent lifecycle](../design/agent-lifecycle.md): how an agent orients by drilling down one level at a time

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-07}
Sprint 57's third Done-when item, "pm prime under 7,000 characters, nothing cut", is met when no session-start hook is cut; the total rules size is not part of it.
The rules alone are about 15,400 characters; the goal was that project state reaches the agent whole, and pm prime --state now prints 2,211 characters, under the 10,000 cap.
Answers `yeeef-agents-9va.66.4`.
:::

::: decision {source=owner date=2026-10-07}
The top level of pm show drops the day summary line; pm show --json and the day page keep it, and no text level prints it.
The owner does not need the day summary at session start, and no other level is a natural home for a line that spans projects.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Top level of `pm show` on this repo: 1,187–1,367 characters (state varied
  between runs), down from 7,975 for the single-level output; `pm prime
  --state` (init, where, show) dropped from 10,041, cut at the 10,000 cap, to
  2,191, nothing cut.

- On this repo's real state, all 86 lines of the old `pm show` (installed pm)
  appear in the new top level plus `--project` levels, except the day summary
  line, which the service regenerated between the two runs.

- The rules alone are about 15,400 characters across two session-start hooks
  (9,390 and 5,982), so "pm prime total under 7,000" cannot hold without
  cutting the rules; no hook is cut today. The top level grows about 90
  characters per open owner request: about 9 more than today pass 1,500.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: `pm show` prints a top level of about 1,300 characters, deeper levels carry the rest, and session start no longer cuts project state.

Merged as 1a649cb (PR #74).

- `pm show` prints the top level only: push and held-task warnings, the day and site lines, one line per open project and one per owner request.
- `pm show --project NAME|ID` prints every line the old output printed for that project; `--sprint`, `--record --section` and `--json` are unchanged.
- `prime.md` tells agents to drill down one level at a time; three design pages describe the levels.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

| Item | Met | Evidence |
|---|---|---|
| Top level at most 1,500 characters, keeping every held-task warning, undelivered reply, open owner request and push failure | met | `pm show \| wc -c` gives 1,303 on this repo (7,975 before); `test_pm_show_levels_together_print_every_line_pm_show_printed_whole` checks each kind of line |
| Every line `pm show` printed is reachable through some level | met | The same test compares, by line counts, the top level plus every project level with `OLD_SHOW`, which the old renderer printed (checked byte for byte by the reviewer); on this repo's real state, 86 of 86 old lines appear, except the day line that the service regenerated between runs. The day summary line then left the text levels on the owner's word; `pm show --json` and the day page keep it |
| `pm prime` total under 7,000 characters, nothing cut | met as the owner read it | Owner decision: the item means no hook is cut. `pm prime --state` prints 2,211 characters (10,041 before, cut at the 10,000 cap); the rules hooks are 9,390 and 5,982, each under the cap |

CI on PR #74: light, integration and guard jobs pass.
