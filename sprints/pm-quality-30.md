---
type: sprint
title: The agents md CI jobs finish within the PR's other checks
bead: pm-d2k5.30
---

## Goal

> What should be true when this sprint ends, and why now?

The `agents md` workflow's two jobs, `check` (grades AGENTS.md) and `cases` (tests the grader on 13 labelled cases), stop setting how long a PR's CI takes, with every verdict unchanged. Why now: on PR #80, `cases` took 275 s and `check` 156 s, the two slowest of its 15 checks; the next was `build-test (darwin-arm64)` at 155 s. `check` in full mode runs on every PR that changes AGENTS.md and `cases` on every PR that changes the grader, so both will recur.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Measure where each job's time goes: checkout, Go build (`cases`), Claude Code install, and the model calls (the slowest call or group of voting calls, queueing behind the concurrency bound).
- Cut it: e.g. the calls' concurrency and how full mode and the cases split their calls, Claude Code's install (cache or pinned binary), the Go build the `cases` job runs only for `conftest.py`, the order calls start (slowest first).
- Hold the verdicts: the rubric, the voting rule and the case labels stay as they are.

**Out:**
- A different model or rubric.
- Diff mode's speed (already 10-40 s of grading).
- The other workflows' jobs.

## Done when

> What evidence will show the goal is met?

- On a PR that changes the grader and AGENTS.md, `check` and `cases` each finish no later than that PR's slowest other check, or under 155 s (PR #80's slowest other check) if the other checks are faster; job times from `gh pr view --json statusCheckRollup`.
- The calibration gives 13/13 in CI and locally, and a clean full run passes, with per-run cost no more than 10% above PR #80's ($0.27 for `check`, $0.68 for `cases`).
- `make test`, `make lint` pass, and the PR's CI is green.

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

None yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
