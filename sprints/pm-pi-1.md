---
type: sprint
title: pm knows a pi session
bead: pm-puvw.1
---

## Goal

> What should be true when this sprint ends, and why now?

pm resolves the session it runs in when pi runs it, and counts a pi session as live, so every later sprint has a runtime name and a session id to build on. pi exposes `PI_SESSION_ID`, `PI_SESSION_FILE` and `AI_AGENT=pi` to the commands it runs, and keeps each session in a JSONL file under `~/.pi/agent/sessions/`.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- `pi` as a third runtime in `internal/session`: runtime detection, session id, nesting with Claude Code and Codex.
- Liveness from the pi session file, and the worktrees a pi session used, for `pm show` and `pm clean`.
- The pi version pm supports, pinned and named in the design page.

**Out:**
- Installing anything into pi (sprint 2), the ask tool and reply delivery (sprint 3), the judge on pi (sprint 4).

## Done when

> What evidence will show the goal is met?

- Go unit tests: `session.Current` resolves pi alone, and pi nested inside Claude Code and inside Codex.
- Live check in a scratch clone: a pi session that runs `pm show` shows up as a live session with its pi session id.

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
