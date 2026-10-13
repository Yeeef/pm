---
type: sprint
title: Ask each session for pm feedback when it ends
bead: pm-d2k5.25
---

## Goal

> What should be true when this sprint ends, and why now?

Each agent session that used pm reflects on its pm experience once, when the session ends, and adds `pm feedback add` entries for what got in its way, or writes nothing. The session itself cannot act once it ends (SessionEnd cannot block or reach the model), so a SessionEnd hook resumes a fork of the ended session, which holds the agent's whole context, and asks it. Why now: 31 feedback entries in 7 days came only from agents that volunteered them mid-task; friction they worked around is never reported.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- A SessionEnd hook entry, Claude Code and Codex, that returns at once and starts a detached run: `claude -p --resume <id> --fork-session`, or `codex exec resume <id> --ephemeral`, from the session's cwd, with the feedback prompt (in `prompts/`) and hooks off.
- Gated: only a session that ran a pm command, only once per session (a marker under `.pm/run/`), never in the forked run itself.
- The run's output logged under `.pm/run/`; a missing cwd or a failed run logs its cause and writes nothing.
- Tests against the fake runtimes, and a live check on both runtimes.

**Out:**
- Turn-end (Stop) asks.
- A refusal log; the forked run has the session's own context.
- Changes to `pm feedback add` itself.
- Sessions that end without SessionEnd firing (killed, crashed).

## Done when

> What evidence will show the goal is met?

- Harness tests: a session that ran pm starts one feedback run with the right resume command, cwd and prompt, and hooks off; a second SessionEnd for the same session, a session with no pm command and the forked run itself start none; the hook returns within its timeout.
- `TestClaudeAndCodexRunTheSameHooks` passes with the new entry in both runtimes.
- A live check in a scratch clone on Claude and Codex: a session that hit a pm refusal ends, and its forked run adds a feedback entry naming it, or the run log says why not; the original transcript gains no turn.
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

::: decision {source=agent date=2026-10-13}
Renamed the sprint from "Ask for pm feedback at Stop only when pm refused something this session" to "Ask each session for pm feedback when it ends": The owner meant session end, not turn end: a Stop hook fires at every turn.
The title now names the session-end design.
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
