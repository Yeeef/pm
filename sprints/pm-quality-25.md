---
type: sprint
title: Ask for pm feedback at Stop only when pm refused something this session
bead: pm-d2k5.25
---

## Goal

> What should be true when this sprint ends, and why now?

pm learns where it gets in agents' way from evidence rather than from unprompted feedback alone. pm logs every refusal and failure of its own commands, and a Stop hook asks a session for `pm feedback add` once, only when that session hit refusals and has not already added feedback. Why now: 31 feedback entries in 7 days came only from agents that volunteered them; refusals nobody reported are invisible.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A refusal log: each pm command that refuses or fails appends its command, error text, session id and time under `.pm/run/`.
- A Stop hook entry (Claude Code and Codex), last in the Stop chain, that blocks once per session when the log has entries for this session and the session ran no `pm feedback add`; its reason quotes the refused commands and says to add feedback only for a refusal that was pm's fault.
- Its interaction with `stop_hook_active`: the owner-request and uncommitted-records checks keep their guarantees, or the gap is measured and named.
- Tests, and a parity live scenario on both runtimes.

**Out:**
- Asking every session for feedback.
- Aggregating or rendering the refusal log on the site (a later sprint, once data exists).
- Changes to `pm feedback add` itself.

## Done when

> What evidence will show the goal is met?

- A harness test: a session with a refused pm command is blocked once at Stop with a reason quoting the refusal; a session with none, one that already added feedback, and the continuation turn are let through.
- `TestClaudeAndCodexRunTheSameHooks` passes with the new entry in both runtimes.
- A parity live run on Claude and Codex: a session that hits a refusal is asked once and either adds one feedback entry or stops; a session with no refusal gets no extra turn.
- The PR is on main.

## Design pages

> Where is the detail?

None yet.

## Progress

> Where is the sprint now? Generated from the work store when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-13}
Ask for pm feedback at Stop only when this session hit a pm refusal, once per session, never every session
Asking every session would bury the 31 unprompted entries under made-up feedback, and every Stop block opens a stop_hook_active gap in the owner-request and uncommitted-records checks; gating on refusals keeps the extra turn rare and backed by evidence
:::

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
