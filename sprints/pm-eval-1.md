---
type: sprint
title: Survey long-horizon agent benchmarks pm could be measured on
bead: pm-itx2.1
---

## Goal

> What should be true when this sprint ends, and why now?

A survey of public long-horizon agent benchmarks and evals exists, with each one judged on whether it can show pm's effect, and a ranked shortlist with a recommended first benchmark. Now, because every later sprint of pm-eval (harness, runs, analysis) depends on which benchmark is chosen.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Public benchmarks and evals of long-horizon or multi-session agent work: coding (e.g. SWE-bench variants, long-running repo tasks), terminal and tool use, research and web tasks, memory and multi-session continuity.
- For each: what it measures, task length in steps or hours, how many tasks, harness and runtime it needs (Claude Code, Codex, a custom loop), cost per run, licence and access, and how pm would plug in (sessions, hooks, a work store across sessions).
- Where pm's mechanisms (records, a work store, sprint frames, handoff across sessions and context resets) could plausibly move the score, and where they cannot.
- Prior work measuring planning, memory or task-tracking scaffolds on these benchmarks.

**Out:**
- Building a harness or running any benchmark.
- Any code change in this repo.

## Done when

> What evidence will show the goal is met?

- A doc in this project lists at least 10 candidate benchmarks in one table with the dimensions above, each with a source link.
- The doc ranks a shortlist of at most 3, names one recommended first benchmark with its reason, and states the with-pm vs without-pm design it implies and an estimated cost per full comparison.
- The recommendation goes to the owner as a decision need.

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

- 24 candidate rows (about 29 benchmarks) surveyed, each with a source link.
  Only SWE-Marathon (20 tasks, 2-10 h agent limit, Apache-2.0, Claude Code and
  Codex built in through Harbor) has tasks that outlast one context window and
  one session on both runtimes; its paper reports 31.4% timeouts, 7.6%
  premature stops and compaction correlated with failure.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
