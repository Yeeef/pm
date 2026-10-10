---
type: sprint
title: Sessions never duplicate each other's work
bead: yeeef-agents-9va.27
---

## Goal

> What should be true when this sprint ends, and why now?

Two sessions never start the same work without knowing it. On 2026-10-05 the main session delegated sprint 21 while another session (462bd145) already held its task 9va.25.1 and had raised two owner decisions under it; the duplicate ran about 6 minutes before the owner noticed.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:** study why it happened: claims carry only the shared git user "yeeef", not the session; `pm show` shows a sprint as running but not by whom; the delegating agent did not check claims before briefing a subagent; subagents claim with the same identity and refresh expired leases. Then fix the gaps, for example claims that record the session, `pm show` naming who holds each running sprint or task, a check that refuses or warns when another live session holds the work, and a rule that briefs check claims first.

**Out:** coordinating work across machines; replacing Beads claims.

## Done when

> What evidence will show the goal is met?

- A short study (doc) lists how the duplicate happened, with the evidence, and earlier near-misses of the same kind.
- A session that tries to start work another live session holds is told so before it begins, checked for real with two sessions.
- `pm show` names who holds each running sprint and task.

## Design pages

> Where is the detail?

- [Session claims](http://localhost:8000/design/session-claims.html): `pm task claim`, liveness and display.

## Progress

> Where is the sprint now? Generated from Beads when the page is rendered.
> Do not write here.

## Decisions

> What did we choose inside this sprint, and why?

::: decision {source=owner date=2026-10-05}
Build fixes A+B+C+D from the duplicate-work study: pm task claim records the session id; pm show names each running task and sprint's holder and claim age; claims are refused and session start warns when another live session holds the work; RULES.md requires checking holders and open needs before delegating. Skip E (a hook blocking raw bd --claim).
The owner chose the study's default; E costs a hook for a path the other fixes already cover.
Answers `yeeef-agents-9va.27.2`.
:::

## Findings

> What did we learn that changes the design, the plan, or how we work? Add
> results with their numbers.

- 2026-10-05: the main session (86524d12) delegated sprint 21 to a subagent
  without reading claims; another session (462bd145, 'pm: sprint 21') already
  held task 9va.25.1 and had raised needs 9va.25.2 and 9va.25.3. The duplicate
  made uncommitted edits (75 lines added, 37 removed, reverted) and refreshed
  the existing claim on 9va.25.1 under the same assignee 'yeeef'; nothing was
  committed or pushed. The owner noticed.

- Study (http://localhost:8000/docs/2026-10-05-duplicate-session-work.html):
  four root causes. (1) Every claim is assignee 'yeeef', so bd --claim is
  idempotent across sessions and the 5-minute lease on 9va.25.1 had expired 11
  minutes before the second claim (14:40:41 vs 14:51:55 UTC). (2) pm show
  prints 'running' with no holder. (3) Delegation briefs never read claims or
  open needs; the overlap was found 6 minutes later by listing tasks. (4) A
  session id exists (CLAUDE_CODE_SESSION_ID; hook stdin session_id) but
  nothing records it. No earlier same-task duplicate found; the nearest is
  sprint 10's cross-session pm commit sweep.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: claims now record the session, `pm show` names who holds each task and sprint, and a claim on work another live session holds is refused.

- `pm task claim <id>` stores `claimed_by` (session id) and `claimed_at` in the issue metadata; it refuses when another live session (transcript written in the last 30 minutes) holds the task, and takes over an idle one.
- `pm show` prints `[held by <session>, <age>, live|idle]` for each in-progress task, a `held by:` line per sprint, and a warning listing other live sessions' work; the SessionStart hook passes its session id so that warning reaches a new session.
- RULES.md and SKILL.md: claim with `pm task claim`; check holders and open needs in `pm show` before starting or delegating.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Study doc with evidence and near-misses: met; docs/2026-10-05-duplicate-session-work (timeline, four root causes, no earlier same-task duplicate found).
- A session starting work another live session holds is told so, checked with two sessions: met; on [TEST] task 9va.27.7, session 1bdea93a claimed it, then `CLAUDE_CODE_SESSION_ID=2222… pm task claim` exited 1 with "held by live session 1bdea93a… leave it, or ask that session", and `pm show` under that id warned the same. Raw `bd update --claim` still bypasses the check (option E, not chosen).
- `pm show` names who holds each running sprint and task: met; PR #27 merged as fe41a41, `uv run tests/run.py` 310 passed, `make render` 48 pages. Claims made before the change show as "held by yeeef without a session".
