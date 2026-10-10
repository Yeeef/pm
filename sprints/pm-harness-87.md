---
type: sprint
title: "Go port: agent commands, part two (P6)"
bead: yeeef-agents-9va.96
---

## Goal

> What should be true when this sprint ends, and why now?

The commands that carry the owner's needs and replies run on Go pm with Python's behaviour, so every agent-facing command exists in Go before install and release.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** `decision add/need/close`, `action need/done` (with `--pr`), `reply read`, `day summarize`, `hook owner-request`; the refusal-string check for these commands.

**Out:** the service's inbox push and merge poll (service sprint); install, launcher and release.

## Done when

> What evidence will show the goal is met?

- `test_pm.py` and `test_owner_request_hook.py` pass with `PM_IMPL=go`.
- Their differential transcripts equal Python's after normalisation.
- The sprint's PR is on main.

## Design pages

> Where is the detail?

- [pm in Go](../design/pm-go.md): Tests, Port order and coexistence

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Shared suite on Go pm after the port (CI make test-go-suite, 9e6cc45, macOS
  and Linux): 77 passed, 41 xfailed, 186 skipped; 77 transcripts equal to
  Python's (sprint 86 ended at 47 passed, 70 expected failures).
  go-expected-failures.txt: 71 entries before, 42 after; test_pm.py and
  test_owner_request_hook.py went from 39 entries to 10, all of them the pm
  service's site replies and inbox push, pm push and pm init on a fresh clone,
  which the service sprint and install ports own. Locally on 2bbf21a:
  test_pm.py, test_owner_request_hook.py and test_go_refusals.py on Go 61
  passed, 10 xfailed, 3 skipped; compare_transcripts 61 compared, 0 differ;
  make test 114 passed. Static refusal check: 17 more Python functions in
  PORTED.

- Differences by the work-store design that the port surfaced: (1) answering a
  need: Python's bd human respond writes a note 'Response: <text>' by the bd
  user, Go's work.Answer a reply by the owner; tests/transcript.py gives both
  one form, and the fake bd's comments now carry UUIDs as bd's do. (2)
  Answer's reply counted as an undelivered owner reply, so pm reply read <id>
  on an answered need printed the agent's own decision (fresh-context review,
  fixed in 2bbf21a: only replies made before the close count). A mark-only
  rule was rejected: this clone's bd export holds 93 site replies without the
  pm-reply mark and 78 with it. (3) Go prints Python's undo hint (bd update …
  --remove-label=no-decision) after pm decision close for transcript parity;
  the work store has no such command, so the text must change at the cut-over.
  (4) A need raised with an inbox but no session keeps no inbox in Go
  (RaisedBy needs a session), as the bd import maps it.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

done: Go pm runs decision add/need/close, action need/done (with `--pr`), reply read, day summarize and hook owner-request with current main's Python behaviour.

- The commands that carry the owner's needs and replies; the owner-request hook and the day summary, their prompts shared by both implementations.
- An answer recorded by a session is not delivered back as an owner reply (a review fix, with its test).
- Merged by the agent session under the owner's project decision that it merges the Go port PRs.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Met, for this sprint's scope:** `test_pm.py` and `test_owner_request_hook.py` pass with `PM_IMPL=go` except 10 tests that need the pm service, `pm push` or `pm init` on a fresh clone, which Scope leaves to the service and install sprints; the shared suite on Go: 77 passed (47 before), expected failures 71 → 42.
- **Met:** their transcripts equal Python's after normalisation: 77 compared, 0 differ (CI on `2bbf21a`).
- **Met:** the PR, [#93](https://github.com/Yeeef/yeeef-agents/pull/93), is on main as `e60f3e0`.
