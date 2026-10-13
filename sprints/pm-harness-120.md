---
type: sprint
title: Opening a project, sprint or task the owner asked for needs no leave
bead: yeeef-agents-9va.130
---

## Goal

> What should be true when this sprint ends, and why now?

An agent opens the project, sprint or task that holds work the owner asked for without asking leave, as it already
commits, pushes and opens PRs. Today only prime.md's push rule and the owner-request judge's "authorized" kind cover
commit/push/PR; "Shall I open a sprint for this?" after the owner asked for the work is not caught. The owner asked
for the rule in chat on 2026-10-13.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- The rule in its three texts, in one commit: prime.md's invariant, the judge prompt's "authorized" kind, the needless block reason.
- Labelled cases in `tests/owner_request_cases.json` for each case the rule moves, and ones it must not move (opening work nobody asked for stays a decision; asking what the goal is stays a clarification).
- A changelog entry.

**Out:**
- Any change to `pm project open`'s goal rule beyond saying the owner's request can state the goal.
- Code changes to the hook's handling of "authorized".

## Done when

> What evidence will show the goal is met?

- `make test` passes and `make test-live` passes every case on both runtimes (3 runs each), counts in Findings.
- The PR's CI is green and its review is raised with `pm action need --pr`.

## Design pages

> Where is the detail?

- [Owner-request Stop hook](../design/owner-request-hook.md): item 4 of the rule and the prime.md row name opening the project, sprint or task for work the owner asked for.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-13}
Opening a project, sprint or task for work the owner asked for needs no leave, like commit, push and PR.
The owner asked for this rule in chat on 2026-10-13.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- make test-live on the new rule, 74 cases x 3 runs: Claude 222/222, median
  2.35 s, p90 4.14 s; Codex 219/222, median 5.37 s, p90 13.96 s: three old
  id-citing cases missed once each. origin/main's prompt misses them too (8
  runs of the 7 id cases on Codex: 53/56 on main, 54/56 with the rule). A
  probe of raw Codex answers: 'Please decide 9va.15.3' unmatched 7/16 (the
  listed need has another id), a PR review cited by id with no open requests
  read as pending 2/16.

- After two judge prompt fixes (Step 3: a match needs no id; the review kind
  defers to the citation rule), the probe gives 16/16 and 24/24 on those
  cases, and make test-live, 74 cases x 3 runs: Claude 222/222, median 2.32 s,
  p90 4.02 s, max 11.82 s; Codex 222/222, median 5.62 s, p90 13.78 s, max
  18.76 s. The six new cases passed 3/3 on both runtimes in both runs.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
