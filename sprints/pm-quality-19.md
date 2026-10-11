---
type: sprint
title: Lint gates stop sloppy code patterns in CI
bead: pm-d2k5.19
---

## Goal

> What should be true when this sprint ends, and why now?

Sloppy patterns the review found cannot come back: CI fails on them. Today only `go vet` and `gofmt` gate. On origin/main, a candidate golangci-lint config finds 137 findings in linters that can gate now and 473 that need a ratchet. deadcode finds 6 unused functions and ruff finds 107 findings in the harness.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `.golangci.yml` with these linters gating now: staticcheck, unused, ineffassign, wastedassign, unparam, nilerr, errorlint, exhaustive, forbidigo (`time.Sleep` in tests), bodyclose, thelper, usetesting, gocritic diagnostics. Their findings are fixed, or kept with a `//nolint` that gives the reason.
- These linters ratcheted on new code (`new-from-merge-base`): errcheck with check-blank, gocyclo over 30, gocognit over 40, funlen at 100 lines, a 1000-line file cap, reassign.
- deadcode as a gate.
- ruff on the harness, including the `time.sleep` ban.
- A package size cap test.
- A test that every flag `prime.md` and the `--help` texts name actually parses.
- A `make lint` target, and a CI job running it.

**Out:**
- Clearing the ratcheted backlog. The test-trim and cli-split sprint does part of it.

## Done when

> What evidence will show the goal is met?

- `make lint` passes on the PR, and CI runs it.
- Re-adding an ignored error (`x, _ := strconv.Atoi`) or a `time.Sleep` in a test fails the lint job: shown on a scratch commit.
- Lint adds under 90 s to CI's critical path.

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

- golangci-lint v2.14.0 on eedbafd: 118 findings across the gating linters
  (uniq-by-line off). On this branch the gating pass and the ratchet pass both
  give 0 issues. Of the 118: about 85 fixed in code; 27 kept with //nolint and
  a reason (8 nilerr fail-open returns, 6 usetesting short socket dirs, 4
  forbidigo sleeps where there is no condition to wait on: a timed hold,
  writer pacing, Dolt's wall-clock read dedup; 2 ST1005 Python messages, 1
  weakCond); 2 nilerr sites fixed (clone.Sparse, service.Uninstall); 7
  sprintfQuotedString and 5 QF1001 by config, with the reason in
  .golangci.yml.

- Ratcheted backlog on eedbafd, left for later sprints: errcheck check-blank
  404 (223 in tests), gocyclo>30 19, gocognit>40 21, funlen 26,
  file-length>1000 3, reassign 19. New code is gated through
  --new-from-merge-base origin/main. Fixed now: the known blank errors in
  sync/push.go, cli/writes.go (x2) and service/spool.go.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Not closed yet.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

Not closed yet.
