---
type: sprint
title: Stop hook judge runs on Haiku 5.5
bead: yeeef-agents-9va.71
---

## Goal

> What should be true when this sprint ends, and why now?

The owner-request Stop hook's judge runs on Claude Haiku 5.5 instead of Haiku 4.5, with the same verdicts on every labelled case.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the `MODEL` constant in `harness/owner_request_hook.py`.
**Out:** the day-summary model (`pm day summarize` uses the `haiku` alias); the judge prompt.

## Done when

> What evidence will show the goal is met?

- `owner_request_hook.py` names `claude-haiku-5-5`.
- `make test-live` passes every labelled case 3/3 with the new model.
- One judge call stays well under the 15 s judge timeout.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

None yet.

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- Haiku 5.5 gives every labelled owner-request case its expected verdict 3/3 (`make test-live`, 35 passed in 31 s).
- One judge-style `claude -p` call on Haiku 5.5 took 0.8, 2.5 and 1.3 s.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: the owner-request Stop hook judge runs on `claude-haiku-5-5` (PR #64).

Merged as 90ae30b (PR #64).

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- `owner_request_hook.py` names `claude-haiku-5-5`: met, commit 5dbf347.
- `make test-live` passes every case 3/3: met, 35 passed.
- One judge call well under 15 s: met, 0.8–2.5 s over three calls.
