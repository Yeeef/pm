---
type: sprint
title: Heavy integration tests run only in CI
bead: yeeef-agents-9va.70
---

## Goal

> What should be true when this sprint ends, and why now?

Agents run only light tests while they work, and the heavy integration tests run in CI on every PR, so a test run never costs an agent minutes or flakes under the load of parallel sessions. Why now: sprint 45 added tests that start real services, ports and homes, and two tests failed only under load on 2026-10-07.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** an `integration` marker on every test in `pm/tests` that starts real processes, ports, temporary homes, clones, remotes, the site server or the service; `make test` runs the rest; a GitHub Actions workflow that runs the integration set (and the light set) on each PR; the rules saying agents run only the light set and raise a PR review once CI passes, in `pm/AGENTS.md` and this repo's rules.
**Out:** the live owner-request judge tests (they need Claude API access); rewriting command tests as in-process unit tests.

## Done when

> What evidence will show the goal is met?

- `make test` runs no test that starts a real process, port or service, shown by a check that fails on an unmarked one; it passes in at most 10 s on the Mac. Expected: about 30 tests.
- A PR to `main` gets a CI run that runs the integration set and reports pass or fail on the PR. Expected: green on the branch that carries this sprint.
- The rules name `make test` for local work and a green CI run before `pm action need --pr`.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=agent date=2026-10-07}
Rename Sprint 59's slow marker to integration and widen it, rather than add a second marker; each helper that starts serve, the service, a clone or a remote refuses a test not marked integration; CI runs on Linux only; a green CI run replaces the rule to run make test-full before review, and make test-full stays to reproduce CI locally.
One marker avoids two near-identical sets; a guard in the helpers is exact and cheap; the tests fake bd, gh, claude and the schedulers, so a Linux runner needs only uv and git, and macOS minutes cost ten times as much on a private repo.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Before: make test ran 44 tests in 10.3-14.9 s, because 3 light tests each
  rendered the whole site (2.8-4.0 s each); marking them integration brought
  it to 41 tests in 7.5-8.9 s at load 2-9.

- CI on ubuntu-latest, PR #66: light job 17 s (41 passed), integration job 47
  s (29 passed); the tests need only uv and git there, since bd, gh, claude
  and the schedulers are faked.

- The unmarked-test guard fails 3 tests and errors 4 fixtures when their
  markers are removed, naming what each started (pm service, git push, pm
  init).

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: `make test` runs only pm's 41 light tests, in 6.9-8.9 s, and the 29 integration tests run in CI on every PR and push to main.

Merged as 74015a9 (PR #66).

- Commit eaac080 renames the `slow` marker to `integration`, marks the 3 tests that render the whole site, and adds an autouse guard that fails an unmarked test starting heavy work.
- Commits d3b2bb5 and 72b3515 make `make test` light-only, add `ARGS` for `-k` runs, add `.github/workflows/pm-tests.yml` with separate light and integration jobs, and write the rules in `pm/AGENTS.md`.
- Commit afa56dc closes the review's gaps: the guard also recognizes `python -m pm.cli` and plain `pm prime`; CI gets a 10-minute timeout, read-only permissions and no cancelled runs on main.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- `make test` runs no heavy test, with a check that fails on an unmarked one, in at most 10 s: met. 41 passed in 6.9 s (twice, load ~2-5) and 7.8-8.9 s (load 7-9); removing markers from 4 tests and one module failed or errored all 7 affected tests, each naming what it started.
- A PR gets a CI run of the integration set that reports on the PR: met, against `main` (the sprint now targets main, since `pm-package` merged). PR #66, run 37678188433: light 17 s (41 passed), integration 37 s (29 passed).
- The rules name `make test` for local work and green CI before `pm action need --pr`: met. `pm/AGENTS.md` Tests section, which also says to run only the touched integration tests with `-k` while iterating.
