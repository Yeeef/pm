---
type: sprint
title: "Day summary: at most four themes of important changes"
bead: yeeef-agents-9va.109
---

## Goal

> What should be true when this sprint ends, and why now?

The day summary reads as a short briefing, not a task list: at most four themes, each with only the changes that matter to the owner. Now the uncapped prompt lists every task, so a busy day gives about 20 bullets.

## Scope

> What's in, and what's explicitly out? Keep this high level; implementation
> details go in a design page.

**In:**
- Rewrite `SUMMARY_PROMPT` in `pm/src/pm/cli.py`: at most four theme bullets, `- **Theme:** ...`, important changes only.
- Its test, if one checks the prompt text.

**Out:**
- Changing how the site renders the summary, or `summary_line` (Python and Go).
- Re-generating past summaries.
- The pm release that carries the change to the service.

## Done when

> What evidence will show the goal is met?

- `pm day summarize --dry-run` for 2026-10-08 with the new prompt prints at most four bullets, each starting with a bold theme, and no task-level detail.
- The pm tests pass.

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

- The new prompt, run on real activity with nothing written, gave 4 themes for
  2026-10-08 (29,910 characters of activity; the uncapped prompt gave 21
  bullets for this day in sprint 95's dry run) and 4 themes for 2026-10-07
  (61,031 characters), each a bold theme with one or two sentences and the
  owner's requests last as For you.

## Delivery report

> Written at close. Each part holds "Not closed yet." until then.

### Outcome

> Done, partial or voided, plus one sentence; then, optionally, bullets of what shipped.

Done: the day summary prompt now asks for at most four themes, each with only the changes that matter to the owner.

Merged as 03e36ae (PR #89).

- `SUMMARY_PROMPT` in `pm/src/pm/cli.py`: one `- **Theme:** ` bullet per theme, at most four; open owner requests are the last theme, For you.
- The pm service uses it after the next pm release.

### Against "Done when"

> Each item, met or not, with its evidence (a page, a command, a number).

- Dry run on 2026-10-08 prints at most four bold-theme bullets with no task-level detail: **met**. The new prompt on that day's real activity (29,910 characters) gave 4 themes; on 2026-10-07 (61,031 characters) it also gave 4. Both are quoted in the PR.
- The pm tests pass: **met**. `make test`: 109 passed, 136 skipped; the PR's CI (light, integration, Go parity on macOS and Linux) is green.
