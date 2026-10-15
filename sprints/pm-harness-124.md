---
type: sprint
title: macOS Go jobs run once a day, not on every PR and push
bead: yeeef-agents-9va.134
---

## Goal

> What should be true when this sprint ends, and why now?

pm-go.yml's two macOS jobs (build-test and race on macos-14) run once a day on main instead of on every pull request and every push to main. macOS runners are the slowest and scarcest jobs in pm's CI (GitHub runs at most 5 macOS jobs at once per account), and the owner asked for them to run daily.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- pm-go.yml: the macOS matrix rows run only on a daily schedule (and on manual dispatch), on main, running every test (the push-to-main `run`).
- PRs and pushes to main run only the Linux rows; merge_ready.py and the CI tests (tests/test_ci.py) agree with the new set of required jobs.
- The workflow's header comment and any doc that says macOS runs on every PR.

**Out:**
- pm-release.yml and pm-release-build.yml macOS builds (they run on tags and release-build changes only).
- Other repos' CI.

## Done when

> What evidence will show the goal is met?

- A PR's CI shows no macOS job and is green; `make merge-ready` accepts it.
- `gh workflow run pm-go.yml --ref main` (or the schedule) runs the macOS jobs on main and they pass, after merge.

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

- PR #98 CI on cf6c9e2: every Linux job passed; the macOS job shows as skipped
  on the PR. make merge-ready PR=98 accepted it. The PR-only macOS test subset
  (go-os-tests.txt, go_os_tests.py, go-os-check) is removed: 8 files changed,
  +121 -475 lines. No macOS run has happened under the new schedule yet.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
