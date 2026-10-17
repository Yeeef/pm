---
type: sprint
title: The owner-request judge and parity eval on pi
bead: pm-puvw.4
---

## Goal

> What should be true when this sprint ends, and why now?

The owner-request judge runs on pi when the session is a pi session, the request-rule texts name pi's ask tool, and the parity live eval runs its scenarios on pi as on Claude Code and Codex, so pi's guarantees are measured, not assumed.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `ask.Pi` (`pi -p` with a pinned provider and model, no extensions), `--runtime pi`, `SummaryRuntimes`.
- `request_rule_autopilot.txt`, `owner_request_system.txt` and the block reasons name pi's ask tool, as one rule change.
- `pi` in the parity eval's runtimes, with a `tests/fake_pi.py` for the light tests.

**Out:**
- Providers other than the one the project decision picks.

## Done when

> What evidence will show the goal is met?

- `make test-live` passes every case on pi, with pass counts and latency in Findings.
- `make test-live-parity` passes on pi.

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
