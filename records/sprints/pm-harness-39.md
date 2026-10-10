---
type: sprint
title: Session-start state is read as a timed snapshot
bead: yeeef-agents-9va.45
---

## Goal

> What should be true when this sprint ends, and why now?

An agent never reports project state to the owner from the `pm show` snapshot injected at session start; the snapshot states when it was taken, and the rules say state given to the owner comes from a fresh `pm show`.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** the session-start hook's header (`harness/session_context_hook.py`) naming the UTC time the snapshot was taken and that it is for orientation only; the rule in `harness/RULES.md` (and the skill's docs that describe the hook) changed from "read it there before running `pm show` again" to "orient from it; re-run `pm show` before stating project state to the owner"; the hook's tests.

**Out:** dropping the session-start injection; refreshing the snapshot during a session; the Stop hook's owner-request prompt (its false positive on a plain offer is recorded as a finding only).

## Done when

> What evidence will show the goal is met?

- A session-start context from the hook begins with a header naming the snapshot's UTC time and saying to re-run `pm show` before stating project state; a test checks it.
- `RULES.md` and every doc describing the hook say the same; a grep finds no "read it there before running `pm show` again".
- `make test` passes.

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

- 2026-10-06: hours into a session, the agent told the owner sprint 15 was
  still open, quoting the session-start pm show snapshot; sprint 15 had been
  closed meanwhile by another session. bd prime carries no issue status, so
  the snapshot was the only state source. Same day the Stop hook's Haiku
  prompt blocked a plain offer ('Want me to open a sprint for that?') as an
  un-cited sprint request, its first observed false positive; out of scope
  here.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: the session-start `pm show` arrives stamped as a timed snapshot, and the rules say state given to the owner comes from a fresh `pm show`.

- PR #44 (merged as 156a82a): the hook's header with the UTC time and the re-run instruction; the matching RULES.md rule; the hook test.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- **Header names the snapshot's UTC time and says to re-run `pm show`; a test checks it: met.** A real hook run began "Project state from `bin/pm show` at session start, 2026-10-06 01:40 UTC: a snapshot to orient by, which other sessions may have changed since; run `bin/pm show` again before stating project state to the owner."; `test_session_start_injects_pm_show` matches that form and checks the body is `pm show`'s output.
- **RULES.md and every doc describing the hook say the same: met.** RULES.md's session-start rule now says so; a grep outside records finds 0 "read it there before running", and no other doc describes the hook's header.
- **`make test` passes: met.** 374 passed, 7 skipped.
