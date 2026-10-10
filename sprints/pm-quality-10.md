---
type: sprint
title: The Stop hooks block only what is the agent's to fix
bead: pm-d2k5.10
---

## Goal

> What should be true when this sprint ends, and why now?

The owner-request judge passes the non-requests agents reported, and does so on labelled cases that `make test-live` measures. `pm hook stop` no longer blocks a parent over a record that its running subagent is writing.

None of the reported sentences is in `tests/owner_request_cases.json` (40 cases), so the current judge's verdict on them is unmeasured:
- a stated plan (pm 2026-10-07 03:43);
- how-to instructions that answer the owner's own question (pm 2026-10-08 00:01);
- a "still waiting on" status line about the agent's own subagent (pm 2026-10-08 02:45);
- conditional facts (pm 2026-10-08 21:47);
- a line reporting a need held in another clone (pm 2026-10-10 03:56);
- a summary restating open needs raised by an earlier session (formal-methods 2026-10-10 03:31).

The stop-hook case is formal-methods 2026-10-07 20:45.

Raised by the pm feedback triage of 2026-10-10 across pm, formal-methods, ai-safety and yeeef-agents.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Each reported sentence becomes a labelled case, its name citing the entry. Its label comes from the request rule in prime.md, never from the judge's answer.
- Where the rule does not settle a label, raise a decision need before labelling. Two cases need one: instructions that answer the owner's own question, and a restated need that is not in the judge's list.
- Decide whether the judge's OPEN REQUESTS list also carries the clone's other open needs, marked as another session's, so a restatement can be checked against them. Today the list holds only this session's needs (`internal/cli/ownerrequest.go`).
- Change the prompt and the block texts until every case passes. One rule lives in three texts, so change them in one commit.
- `pm hook stop` leaves out a path named only in the prompt of a subagent call that has not returned.

**Out:**
- Reading needs from other clones on the machine.
- Codex (pm-codex sprint 1).
- Checking AskUserQuestion calls.

## Done when

> What evidence will show the goal is met?

- `tests/owner_request_cases.json` holds at least 7 new cases, one per reported sentence.
- `make test-live` with `PM_LIVE_RUNS=3` passes every case 3/3, the new cases and all 40 existing ones, the chat-request block cases included. Pass counts and latency go in Findings.
- A harness test of `pm hook stop` shows no block for a path named only by a subagent call that has not returned, and a block for the same path edited by the session's own tool call.
- `make test` and the PR's CI pass.

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
