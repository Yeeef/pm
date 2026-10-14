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

::: decision {source=agent date=2026-10-14}
Sprint 30 (CI time) runs after sprint 33 (stable verdicts).
Both change tests/agents_md/evaluate.py and agents-md.yml, and 33 fixes (vote in diff mode, cases off unrelated PRs, no clean-case repeat of check) also cut the time 30 measures; 30 then measures what remains.
:::

::: decision {source=owner date=2026-10-15}
The model-calling grading leaves PR CI: PRs run the AGENTS.md check in diff mode only; full-mode grading and the labelled cases run in a daily scheduled CI job on main, only when AGENTS.md or the grader changed since its last successful run.
Owner chose this in chat over tuning concurrency: both jobs take 190-300 s on a grader or AGENTS.md change, mostly model calls, and a regression found the day after merge is acceptable.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- After sprint 33, on grader-changing PRs (agents-md-stable, ask-tool-on-site;
  3 runs): check takes 187-241 s, of which grading is 184-226 s and Claude
  Code install 9-15 s; cases takes 262-302 s, of which the cases step is
  219-230 s, setup-go 15-22 s, install 10-11 s. The model calls set both jobs'
  time; setup is at most 35 s.

- Full mode (90 calls, 5 votes): per call median 15-17 s, p90 38-45 s; the
  consistent criterion median 73-131 s, max 102-154 s (one local call: 95 s,
  24 sequential tool turns). A 16-slot replay of 4 runs' call times matches
  the grading step within 1 s (184/184, 225/226, 204/204, 164/164): consistent
  starts late. Its slowest call is the floor, so full mode on a PR stays at
  about 120-175 s with setup even with ideal ordering. Cost with 5 votes:
  check $0.41-0.46 (the $0.27 baseline was 3-vote), cases $0.66-0.73.

- PR #97 (changes the grader and AGENTS.md): check 38 s in diff mode; changed,
  full and cases skipped; slowest other check build-test (darwin-arm64) 2m9s;
  all 15 checks green (gh pr checks 97).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
